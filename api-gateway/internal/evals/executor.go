package evals

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func (s *Service) ExecutePack(ctx context.Context, tenantID string, req models.TraceEvalRequest) (models.EvalExecution, models.TraceEvalRun, error) {
	packID := firstNonEmpty(strings.TrimSpace(req.PackID), strings.TrimSpace(req.EvalSuite))
	if packID == "" {
		return models.EvalExecution{}, models.TraceEvalRun{}, fmt.Errorf("pack_id is required")
	}
	pack, err := GetPackDefinition(s.packRoot, packID)
	if err != nil {
		return models.EvalExecution{}, models.TraceEvalRun{}, err
	}
	execution := models.EvalExecution{
		PackID:      packID,
		Mode:        firstNonEmpty(strings.TrimSpace(req.Mode), "offline"),
		Status:      "running",
		ReleaseTag:  strings.TrimSpace(req.ReleaseTag),
		TraceIDs:    normalizedTraceIDs(req),
		DatasetRefs: normalizedDatasetRefs(req, pack),
		Attributes:  req.Attributes,
		SampleLimit: req.SampleLimit,
	}
	execution, err = s.store.CreateEvalExecution(ctx, tenantID, execution)
	if err != nil {
		return models.EvalExecution{}, models.TraceEvalRun{}, err
	}

	subjects, policySummary, lineage, err := s.executionSubjects(ctx, tenantID, execution)
	if err != nil {
		execution.Status = "failed"
		execution.Summary = err.Error()
		_, _ = s.store.UpdateEvalExecution(ctx, tenantID, execution)
		return execution, models.TraceEvalRun{}, err
	}

	dimensionScores := map[string][]float64{}
	executionItems := make([]models.EvalExecutionItem, 0, len(subjects))
	for _, subject := range subjects {
		item := executeSubjectAgainstPack(pack, subject, execution.Attributes)
		item.ExecutionID = execution.ID
		storedItem, err := s.store.InsertEvalExecutionItem(ctx, tenantID, item)
		if err != nil {
			execution.Status = "failed"
			execution.Summary = err.Error()
			_, _ = s.store.UpdateEvalExecution(ctx, tenantID, execution)
			return execution, models.TraceEvalRun{}, err
		}
		if err := s.store.InsertEvalEvaluatorResults(ctx, tenantID, execution.ID, storedItem.ID, item.EvaluatorResults); err != nil {
			execution.Status = "failed"
			execution.Summary = err.Error()
			_, _ = s.store.UpdateEvalExecution(ctx, tenantID, execution)
			return execution, models.TraceEvalRun{}, err
		}
		if err := s.store.InsertEvalEvidenceLinks(ctx, tenantID, execution.ID, storedItem.ID, item.EvidenceLinks); err != nil {
			execution.Status = "failed"
			execution.Summary = err.Error()
			_, _ = s.store.UpdateEvalExecution(ctx, tenantID, execution)
			return execution, models.TraceEvalRun{}, err
		}
		storedItem.EvaluatorResults = item.EvaluatorResults
		storedItem.EvidenceLinks = item.EvidenceLinks
		executionItems = append(executionItems, storedItem)
		for _, result := range item.EvaluatorResults {
			dimensionScores[result.DimensionID] = append(dimensionScores[result.DimensionID], result.Score)
		}
	}

	scores, overall, risk, summary := aggregatePackExecution(pack, dimensionScores, subjects)
	run := models.TraceEvalRun{
		TraceID:             firstTraceID(subjects),
		PromptID:            lineage.PromptID,
		PromptVersion:       lineage.PromptVersion,
		PromptEnvironment:   lineage.PromptEnvironment,
		ReleaseTag:          firstNonEmpty(execution.ReleaseTag, lineage.ReleaseTag),
		EvalSuite:           pack.Pack.ID,
		OverallScore:        overall,
		RiskLevel:           risk,
		Summary:             summary,
		PolicyEffectiveness: policySummary,
		Scores:              scores,
	}
	run, err = s.store.InsertEvalRun(ctx, tenantID, run)
	if err != nil {
		execution.Status = "failed"
		execution.Summary = err.Error()
		_, _ = s.store.UpdateEvalExecution(ctx, tenantID, execution)
		return execution, models.TraceEvalRun{}, err
	}

	execution.Status = "completed"
	execution.OverallScore = overall
	execution.RiskLevel = risk
	execution.Summary = summary
	execution.PolicyEffectiveness = policySummary
	execution.RunID = run.ID
	execution.Items = executionItems
	execution, err = s.store.UpdateEvalExecution(ctx, tenantID, execution)
	if err != nil {
		return execution, run, err
	}
	return execution, run, nil
}

func (s *Service) executionSubjects(ctx context.Context, tenantID string, execution models.EvalExecution) ([]executionSubject, models.PolicyEffectivenessSummary, promptLineage, error) {
	subjects := []executionSubject{}
	policySummary := models.PolicyEffectivenessSummary{}
	var lineage promptLineage

	for _, traceID := range execution.TraceIDs {
		trace, itemLineage, err := s.loadTraceEvidence(ctx, tenantID, traceID)
		if err != nil {
			return nil, models.PolicyEffectivenessSummary{}, promptLineage{}, err
		}
		summary := summarizePolicyEffectiveness(trace)
		policySummary = mergePolicySummaries(policySummary, summary)
		if lineage.PromptID == "" {
			lineage = itemLineage
		}
		subjects = append(subjects, executionSubject{
			ItemRef:  traceID,
			ItemType: "trace",
			TraceID:  traceID,
			Input: map[string]any{
				"prompt":           firstNonEmpty(trace.RootSpanName),
				"response":         trace.Status,
				"task_constraints": execution.Attributes,
			},
			Expected: map[string]any{},
			Metadata: map[string]any{},
			Trace:    &trace,
			Policy:   summary,
			Lineage:  itemLineage,
		})
	}

	datasetItems, err := s.resolveDatasetItems(ctx, tenantID, execution.DatasetRefs, execution.SampleLimit)
	if err != nil {
		return nil, models.PolicyEffectivenessSummary{}, promptLineage{}, err
	}
	for _, item := range datasetItems {
		subjects = append(subjects, executionSubject{
			ItemRef:    firstNonEmpty(item.DatasetRef+":"+item.ItemKey, item.ItemKey),
			ItemType:   "dataset_item",
			DatasetRef: item.DatasetRef,
			Input:      item.Input,
			Expected:   item.Expected,
			Metadata:   item.Metadata,
		})
	}

	if len(subjects) == 0 {
		return nil, models.PolicyEffectivenessSummary{}, promptLineage{}, fmt.Errorf("no eval subjects resolved for pack execution")
	}
	return subjects, policySummary, lineage, nil
}

func (s *Service) resolveDatasetItems(ctx context.Context, tenantID string, refs []string, sampleLimit int) ([]models.EvalDatasetItem, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	limit := sampleLimit
	if limit <= 0 {
		limit = 200
	}
	seedItems := []models.EvalDatasetItem{}
	dbItems, err := s.store.ListEvalDatasetItems(ctx, tenantID, refs, limit)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		dataset, err := GetDatasetDefinition(s.datasetRoot, ref)
		if err != nil {
			continue
		}
		for _, item := range dataset.Items {
			seedItems = append(seedItems, item)
			if len(seedItems)+len(dbItems) >= limit {
				break
			}
		}
		if len(seedItems)+len(dbItems) >= limit {
			break
		}
	}
	items := append(seedItems, dbItems...)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func executeSubjectAgainstPack(pack EvalPackDefinition, subject executionSubject, attributes map[string]any) models.EvalExecutionItem {
	evidence := evalEvidence{Subject: subject, Attributes: attributes}
	results := make([]models.EvalEvaluatorResult, 0, len(pack.Evaluators))
	dimensionScores := map[string][]float64{}
	for _, evaluator := range pack.Evaluators {
		if !evaluatorAppliesToSubject(evaluator, subject) {
			continue
		}
		result := executeRegisteredEvaluator(evaluator, evidence)
		results = append(results, result)
		dimensionScores[result.DimensionID] = append(dimensionScores[result.DimensionID], result.Score)
	}

	for _, dimension := range pack.Dimensions {
		if len(dimensionScores[dimension.ID]) > 0 {
			continue
		}
		score := 50.0
		if subject.Trace != nil {
			score = fallbackDimensionScore(pack, dimension.ID, *subject.Trace)
		}
		results = append(results, models.EvalEvaluatorResult{
			EvaluatorID:   "dimension.fallback." + dimension.ID,
			DimensionID:   dimension.ID,
			EvaluatorType: "fallback_dimension",
			Method:        "phase1_fallback",
			Score:         score,
			Severity:      severityForScore(score),
			Status:        "completed",
			Summary:       fmt.Sprintf("Phase 1 fallback score for %s dimension", dimension.ID),
			Details: map[string]any{
				"execution_mode": "phase1_fallback",
			},
		})
		dimensionScores[dimension.ID] = append(dimensionScores[dimension.ID], score)
	}

	scores, overall, risk, summary := aggregatePackExecution(pack, dimensionScores, []executionSubject{subject})
	_ = scores
	return models.EvalExecutionItem{
		ItemRef:          subject.ItemRef,
		ItemType:         subject.ItemType,
		TraceID:          subject.TraceID,
		DatasetRef:       subject.DatasetRef,
		Status:           "completed",
		OverallScore:     overall,
		RiskLevel:        risk,
		Summary:          summary,
		Evidence:         evidence.fields([]string{"prompt", "response", "workflow_trace", "tool_trace", "policy.total_events"}),
		EvaluatorResults: results,
		EvidenceLinks:    evidence.evidenceLinks(),
	}
}

func aggregatePackExecution(pack EvalPackDefinition, dimensionScores map[string][]float64, subjects []executionSubject) ([]models.TraceEvalScore, float64, string, string) {
	scores := make([]models.TraceEvalScore, 0, len(pack.Dimensions))
	var weightedTotal float64
	var weightTotal float64
	for _, dimension := range pack.Dimensions {
		weight := dimension.Weight
		if weight <= 0 {
			weight = 1
		}
		score := averageFloat(dimensionScores[dimension.ID])
		scores = append(scores, models.TraceEvalScore{
			Metric:   dimension.ID,
			Score:    score,
			Weight:   weight,
			Severity: severityForScore(score),
			Summary:  fmt.Sprintf("%s averaged %.2f across %d subjects", dimension.ID, score, maxInt(len(subjects), 1)),
		})
		weightedTotal += score * weight
		weightTotal += weight
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].Metric < scores[j].Metric })
	overall := 0.0
	if weightTotal > 0 {
		overall = clampScore(weightedTotal / weightTotal)
	}
	return scores, overall, riskFromPackThresholds(pack, overall), fmt.Sprintf("%s scored %.2f across %d subjects", pack.Pack.Name, overall, maxInt(len(subjects), 1))
}

func executeRegisteredEvaluator(evaluator map[string]any, evidence evalEvidence) models.EvalEvaluatorResult {
	evaluatorID := strings.TrimSpace(stringValue(evaluator["id"]))
	dimensionID := strings.TrimSpace(stringValue(evaluator["dimension"]))
	evaluatorType := strings.TrimSpace(stringValue(evaluator["type"]))
	method := strings.TrimSpace(stringValue(nestedMapValue(evaluator, "scoring", "method")))
	fields := extractInputFields(evaluator)

	score, summary, details := evaluateMethod(method, evaluatorType, fields, evaluator, evidence)
	return models.EvalEvaluatorResult{
		EvaluatorID:   evaluatorID,
		DimensionID:   dimensionID,
		EvaluatorType: evaluatorType,
		Method:        method,
		Score:         clampScore(score),
		Severity:      severityForScore(score),
		Status:        "completed",
		Summary:       summary,
		InputFields:   fields,
		Details:       details,
	}
}

func evaluateMethod(method, evaluatorType string, fields []string, evaluator map[string]any, evidence evalEvidence) (float64, string, map[string]any) {
	switch method {
	case "subset_binary":
		return evaluateSubset(fields, evidence)
	case "all_required_true", "binary", "required_controls_present", "required_evidence_presence", "mandatory_evidence_check", "mandatory_trigger_check":
		return evaluatePresence(fields, evidence, method)
	case "artifact_score":
		return evaluateArtifactScore(fields, evidence)
	case "decision_accuracy", "decision_correctness", "critical_decision_accuracy", "weighted_decision_accuracy",
		"accuracy", "f1", "macro_f1", "classification_f1", "critical_classification_f1", "weighted_accuracy", "binary_or_partial_credit", "recall_priority", "exact_match", "trace_alignment":
		return evaluateComparison(fields, evidence, method)
	case "bounded_rate", "bounded_failure_rate", "critical_violation_rate", "unsupported_claim_penalty", "penalty_score", "violation_penalty", "bounded_trace_steps", "slo_pass_rate", "bounded_age":
		return evaluateBoundedRate(fields, evaluator, evidence, method)
	case "budget_efficiency_score", "ndcg_at_k", "claim_support_ratio", "semantic_similarity", "rubric_average", "judge_model", "hybrid_structural_and_judge":
		return evaluateQualityLike(fields, evidence, method)
	default:
		return evaluateTypeFallback(evaluatorType, fields, evaluator, evidence)
	}
}

func evaluateSubset(fields []string, evidence evalEvidence) (float64, string, map[string]any) {
	var left, right []string
	if len(fields) > 0 {
		left = toStringSlice(evidence.field(fields[0]))
	}
	if len(fields) > 1 {
		right = toStringSlice(evidence.field(fields[1]))
	}
	if len(left) == 0 && len(right) == 0 {
		return 100, "No field subset difference detected", map[string]any{"subset_ok": true}
	}
	rightSet := map[string]struct{}{}
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	for _, item := range left {
		if _, ok := rightSet[item]; !ok {
			return 0, "Requested fields exceed allowed subset", map[string]any{"subset_ok": false, "requested": left, "allowed": right}
		}
	}
	return 100, "Requested fields stay within allowed subset", map[string]any{"subset_ok": true, "requested": left, "allowed": right}
}

func evaluatePresence(fields []string, evidence evalEvidence, method string) (float64, string, map[string]any) {
	if len(fields) == 0 {
		if evidence.Subject.Trace != nil {
			score := evidenceScore(*evidence.Subject.Trace)
			return score, fmt.Sprintf("%s approximated from trace evidence", method), map[string]any{"execution_mode": "trace_evidence"}
		}
		return 50, fmt.Sprintf("%s has no declared fields", method), map[string]any{"execution_mode": "default"}
	}
	present := 0
	values := map[string]any{}
	for _, field := range fields {
		value := evidence.field(field)
		values[field] = value
		if value != nil && fmt.Sprint(value) != "" && fmt.Sprint(value) != "<nil>" {
			present++
		}
	}
	score := (float64(present) / float64(len(fields))) * 100
	return score, fmt.Sprintf("%d/%d required fields present", present, len(fields)), map[string]any{"values": values}
}

func evaluateArtifactScore(fields []string, evidence evalEvidence) (float64, string, map[string]any) {
	score, summary, details := evaluatePresence(fields, evidence, "artifact_score")
	if freshness, ok := toFloat(evidence.field("freshness_days")); ok {
		if freshness > 90 {
			score -= 25
		} else if freshness > 30 {
			score -= 10
		}
		details["freshness_days"] = freshness
	}
	return clampScore(score), summary, details
}

func evaluateComparison(fields []string, evidence evalEvidence, method string) (float64, string, map[string]any) {
	actual := firstValue(evidence, append(fields, "decision", "response", "final_state", "trace.status")...)
	expected := firstValue(evidence, "gold_decision", "expected_decision", "gold_answer", "expected_outcome", "expected.final_state", "expected.decision")
	if len(fields) >= 2 {
		actual = evidence.field(fields[0])
		expected = evidence.field(fields[1])
	}
	if actual == nil && evidence.Subject.Trace != nil {
		score := compositeScore(*evidence.Subject.Trace)
		return score, fmt.Sprintf("%s approximated from trace quality", method), map[string]any{"execution_mode": "trace_quality"}
	}
	actualStr := normalizeValue(actual)
	expectedStr := normalizeValue(expected)
	score := 0.0
	if actualStr == expectedStr && actualStr != "" {
		score = 100
	} else if actualStr != "" && expectedStr != "" {
		score = similarityScore(actualStr, expectedStr)
	} else {
		score = 50
	}
	return score, fmt.Sprintf("Compared %q to %q", actualStr, expectedStr), map[string]any{"actual": actual, "expected": expected}
}

func evaluateBoundedRate(fields []string, evaluator map[string]any, evidence evalEvidence, method string) (float64, string, map[string]any) {
	actual := 0.0
	if len(fields) > 0 {
		if value, ok := toFloat(evidence.field(fields[0])); ok {
			actual = value
		}
	} else if evidence.Subject.Trace != nil {
		trace := *evidence.Subject.Trace
		actual = float64(trace.ErrorCount+trace.Insights.BlockedSpans) / math.Max(float64(trace.SpanCount), 1)
	}
	threshold := 0.0
	if value, ok := nestedNumber(evaluator, "scoring", "max_value"); ok {
		threshold = normalizeThreshold(value)
	} else if value, ok := nestedNumber(evaluator, "scoring", "max_failure_rate"); ok {
		threshold = normalizeThreshold(value)
	} else if value, ok := nestedNumber(evaluator, "scoring", "min_pass"); ok {
		threshold = normalizeThreshold(value)
	}
	score := 100.0
	if strings.HasPrefix(method, "bounded") || strings.Contains(method, "violation") || strings.Contains(method, "penalty") || strings.Contains(method, "unsupported_claim") {
		if actual <= threshold {
			score = 100
		} else {
			score = clampScore(100 - ((actual-threshold)/math.Max(1-threshold, 0.0001))*100)
		}
	} else {
		score = clampScore(actual * 100)
	}
	return score, fmt.Sprintf("Observed %.4f against threshold %.4f", actual, threshold), map[string]any{"actual": actual, "threshold": threshold}
}

func evaluateQualityLike(fields []string, evidence evalEvidence, method string) (float64, string, map[string]any) {
	if len(fields) >= 2 {
		left := normalizeValue(evidence.field(fields[0]))
		right := normalizeValue(evidence.field(fields[1]))
		score := similarityScore(left, right)
		return score, fmt.Sprintf("%s scored via text similarity", method), map[string]any{"left": left, "right": right, "execution_mode": "phase1_similarity"}
	}
	if evidence.Subject.Trace != nil {
		score := qualityScore(*evidence.Subject.Trace)
		return score, fmt.Sprintf("%s approximated from trace quality", method), map[string]any{"execution_mode": "trace_quality"}
	}
	return 70, fmt.Sprintf("%s used phase 1 neutral quality approximation", method), map[string]any{"execution_mode": "phase1_neutral"}
}

func evaluateTypeFallback(evaluatorType string, fields []string, evaluator map[string]any, evidence evalEvidence) (float64, string, map[string]any) {
	switch evaluatorType {
	case "field_subset_validation":
		return evaluateSubset(fields, evidence)
	case "tool_trace_validation", "trace_cost_efficiency", "policy_interaction_check", "workflow_path_eval", "handoff_path_eval":
		return evaluateQualityLike(fields, evidence, evaluatorType)
	default:
		if evidence.Subject.Trace != nil {
			score := compositeScore(*evidence.Subject.Trace)
			return score, fmt.Sprintf("%s used composite trace fallback", evaluatorType), map[string]any{"execution_mode": "trace_composite"}
		}
		return 60, fmt.Sprintf("%s used phase 1 default fallback", evaluatorType), map[string]any{"execution_mode": "phase1_default"}
	}
}

func extractInputFields(evaluator map[string]any) []string {
	raw := nestedMapValue(evaluator, "input", "fields")
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(fmt.Sprint(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []string:
		return uniqueStrings(typed)
	default:
		return nil
	}
}

func evaluatorAppliesToSubject(evaluator map[string]any, subject executionSubject) bool {
	if ref := strings.TrimSpace(stringValue(evaluator["dataset_ref"])); ref != "" && ref != subject.DatasetRef {
		return false
	}
	return true
}

func normalizedDatasetRefs(req models.TraceEvalRequest, pack EvalPackDefinition) []string {
	refs := uniqueStrings(req.DatasetRefs)
	if len(refs) > 0 {
		return refs
	}
	for _, item := range pack.Datasets {
		for _, key := range []string{"ref", "id"} {
			if value := strings.TrimSpace(stringValue(item[key])); value != "" {
				refs = append(refs, value)
				break
			}
		}
	}
	return uniqueStrings(refs)
}

func mergePolicySummaries(left, right models.PolicyEffectivenessSummary) models.PolicyEffectivenessSummary {
	out := left
	out.TotalEvents += right.TotalEvents
	out.Allows += right.Allows
	out.Denies += right.Denies
	out.Warns += right.Warns
	out.Redacts += right.Redacts
	out.BlockedSpans += right.BlockedSpans
	out.RedactedSpans += right.RedactedSpans
	out.CoveredLLMCalls += right.CoveredLLMCalls
	out.TotalLLMCalls += right.TotalLLMCalls
	out.PreventedFailures += right.PreventedFailures
	if out.TotalLLMCalls > 0 {
		out.CoverageRatio = float64(out.CoveredLLMCalls) / float64(out.TotalLLMCalls)
	}
	return out
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return clampScore(total / float64(len(values)))
}

func similarityScore(left, right string) float64 {
	left = strings.TrimSpace(strings.ToLower(left))
	right = strings.TrimSpace(strings.ToLower(right))
	if left == "" && right == "" {
		return 100
	}
	if left == right {
		return 100
	}
	leftTokens := tokenSet(left)
	rightTokens := tokenSet(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	intersection := 0.0
	for token := range leftTokens {
		if _, ok := rightTokens[token]; ok {
			intersection++
		}
	}
	union := float64(len(leftTokens) + len(rightTokens))
	if union == 0 {
		return 0
	}
	return clampScore((2 * intersection / union) * 100)
}

func tokenSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range strings.Fields(value) {
		if len(token) < 2 {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func normalizeValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstValue(evidence evalEvidence, fields ...string) any {
	for _, field := range fields {
		if value := evidence.field(field); value != nil && normalizeValue(value) != "" {
			return value
		}
	}
	return nil
}

func firstTraceID(subjects []executionSubject) string {
	for _, subject := range subjects {
		if strings.TrimSpace(subject.TraceID) != "" {
			return strings.TrimSpace(subject.TraceID)
		}
	}
	return ""
}
