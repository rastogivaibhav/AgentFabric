package evals

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
)

func defaultPackRoot() string {
	if configured := strings.TrimSpace(os.Getenv("GV_EVAL_PACKS_PATH")); configured != "" {
		return configured
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("deploy", "seed", "eval-packs")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "deploy", "seed", "eval-packs")
}

func scoreTraceWithPack(root string, suite string, trace models.Trace) ([]models.TraceEvalScore, float64, string, string, bool) {
	pack, err := GetPackDefinition(root, suite)
	if err != nil {
		return nil, 0, "", "", false
	}
	if len(pack.Dimensions) == 0 {
		return nil, 0, "", "", false
	}

	evaluatorScores := map[string][]float64{}
	evaluatorCounts := map[string]int{}
	for _, evaluator := range pack.Evaluators {
		dimension := strings.TrimSpace(stringValue(evaluator["dimension"]))
		if dimension == "" {
			continue
		}
		score := runPackEvaluator(pack, evaluator, trace)
		evaluatorScores[dimension] = append(evaluatorScores[dimension], score)
		evaluatorCounts[dimension]++
	}

	scores := make([]models.TraceEvalScore, 0, len(pack.Dimensions))
	var weightedTotal float64
	var weightTotal float64
	for _, dimension := range pack.Dimensions {
		weight := dimension.Weight
		if weight <= 0 {
			weight = 1
		}
		values := evaluatorScores[dimension.ID]
		score := 0.0
		if len(values) > 0 {
			for _, value := range values {
				score += value
			}
			score /= float64(len(values))
		} else {
			score = fallbackDimensionScore(pack, dimension.ID, trace)
		}
		score = clampScore(score)
		scores = append(scores, models.TraceEvalScore{
			Metric:   dimension.ID,
			Score:    score,
			Weight:   weight,
			Severity: severityForScore(score),
			Summary:  fmt.Sprintf("%s dimension scored %.2f across %d evaluators", dimension.ID, score, maxEvalCount(1, evaluatorCounts[dimension.ID])),
		})
		weightedTotal += score * weight
		weightTotal += weight
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].Metric < scores[j].Metric })
	overall := 0.0
	if weightTotal > 0 {
		overall = clampScore(weightedTotal / weightTotal)
	}
	risk := riskFromPackThresholds(pack, overall)
	summary := fmt.Sprintf("%s scored %.2f across %d dimensions", strings.TrimSpace(pack.Pack.Name), overall, len(scores))
	return scores, overall, risk, summary, true
}

func runPackEvaluator(pack EvalPackDefinition, evaluator map[string]any, trace models.Trace) float64 {
	method := strings.ToLower(strings.TrimSpace(stringValue(nestedMapValue(evaluator, "scoring", "method"))))
	evaluatorID := strings.ToLower(strings.TrimSpace(stringValue(evaluator["id"])))
	dimension := strings.ToLower(strings.TrimSpace(stringValue(evaluator["dimension"])))
	tags := normalizeTags(pack.Pack.Tags)

	base := baseScoreForMethod(method, evaluatorID, dimension, tags, trace)
	if maxValue, ok := nestedNumber(evaluator, "scoring", "max_value"); ok {
		maxRate := normalizeThreshold(maxValue)
		actual := 1 - (base / 100.0)
		if actual <= maxRate {
			return 100
		}
		if actual >= 1 {
			return 0
		}
		excess := (actual - maxRate) / math.Max(1-maxRate, 0.0001)
		return clampScore(100 - (excess * 100))
	}
	if maxFail, ok := nestedNumber(evaluator, "scoring", "max_failure_rate"); ok {
		maxRate := normalizeThreshold(maxFail)
		actual := 1 - (base / 100.0)
		if actual <= maxRate {
			return 100
		}
		if actual >= 1 {
			return 0
		}
		excess := (actual - maxRate) / math.Max(1-maxRate, 0.0001)
		return clampScore(100 - (excess * 100))
	}
	if minPass, ok := nestedNumber(evaluator, "scoring", "min_pass"); ok {
		target := normalizeThreshold(minPass) * 100
		if target <= 0 {
			return base
		}
		if base >= target {
			headroom := (base - target) / math.Max(100-target, 1)
			return clampScore(target + (headroom * (100 - target)))
		}
		return clampScore((base / target) * 100)
	}
	return base
}

func baseScoreForMethod(method, evaluatorID, dimension string, tags map[string]struct{}, trace models.Trace) float64 {
	switch {
	case strings.Contains(method, "latency"), strings.Contains(dimension, "latency"):
		return scoreLatency(trace).Score
	case strings.Contains(method, "budget"), strings.Contains(method, "cost"), strings.Contains(dimension, "efficiency"):
		return workflowEfficiencyScore(trace)
	case strings.Contains(method, "policy"), strings.Contains(dimension, "policy"), strings.Contains(method, "compliance"):
		return controlScore(trace)
	case strings.Contains(method, "audit"), strings.Contains(method, "artifact"), strings.Contains(method, "evidence"), strings.Contains(method, "notice"), strings.Contains(method, "presence"):
		return evidenceScore(trace)
	case strings.Contains(method, "boundary"), strings.Contains(method, "allowlist"), strings.Contains(method, "permission"), strings.Contains(method, "authorization"), strings.Contains(method, "transport"), strings.Contains(method, "subset"), strings.Contains(method, "exact_match"), strings.Contains(method, "binary"):
		return boundaryScore(trace, tags)
	case strings.Contains(method, "workflow"), strings.Contains(method, "path"), strings.Contains(method, "handoff"), strings.Contains(method, "escalation"), strings.Contains(method, "routing"), strings.Contains(method, "step"), strings.Contains(method, "coordination"):
		return workflowScore(trace)
	case strings.Contains(method, "attack"), strings.Contains(method, "violation"), strings.Contains(method, "refusal"), strings.Contains(method, "leak"), strings.Contains(method, "secret"), strings.Contains(method, "override"), strings.Contains(method, "abuse"), strings.Contains(evaluatorID, "redteam"), hasTag(tags, "red-team"):
		return securityScore(trace, tags)
	case strings.Contains(method, "ranking"), strings.Contains(method, "semantic"), strings.Contains(method, "judge"), strings.Contains(method, "rubric"), strings.Contains(method, "ground"), strings.Contains(dimension, "accuracy"), strings.Contains(dimension, "completeness"), strings.Contains(dimension, "quality"):
		return qualityScore(trace)
	default:
		return compositeScore(trace)
	}
}

func fallbackDimensionScore(pack EvalPackDefinition, dimension string, trace models.Trace) float64 {
	tags := normalizeTags(pack.Pack.Tags)
	switch {
	case strings.Contains(strings.ToLower(dimension), "latency"):
		return scoreLatency(trace).Score
	case strings.Contains(strings.ToLower(dimension), "policy"), strings.Contains(strings.ToLower(dimension), "compliance"):
		return controlScore(trace)
	case strings.Contains(strings.ToLower(dimension), "risk"), strings.Contains(strings.ToLower(dimension), "security"):
		return securityScore(trace, tags)
	case strings.Contains(strings.ToLower(dimension), "workflow"), strings.Contains(strings.ToLower(dimension), "handoff"), strings.Contains(strings.ToLower(dimension), "coordination"):
		return workflowScore(trace)
	default:
		return compositeScore(trace)
	}
}

func compositeScore(trace models.Trace) float64 {
	return clampScore((qualityScore(trace) * 0.35) + (controlScore(trace) * 0.25) + (workflowScore(trace) * 0.20) + (scoreLatency(trace).Score * 0.10) + (workflowEfficiencyScore(trace) * 0.10))
}

func qualityScore(trace models.Trace) float64 {
	reliability := scoreReliability(trace).Score
	evidence := evidenceScore(trace)
	control := controlScore(trace)
	return clampScore((reliability * 0.55) + (evidence * 0.20) + (control * 0.25))
}

func workflowScore(trace models.Trace) float64 {
	reliability := scoreReliability(trace).Score
	latency := scoreLatency(trace).Score
	efficiency := workflowEfficiencyScore(trace)
	score := (reliability * 0.45) + (latency * 0.20) + (efficiency * 0.35)
	if trace.Insights.MaxDepth > 8 {
		score -= 10
	}
	return clampScore(score)
}

func workflowEfficiencyScore(trace models.Trace) float64 {
	cost := scoreCostEfficiency(trace).Score
	score := cost
	if trace.SpanCount > 40 {
		score -= 12
	} else if trace.SpanCount > 20 {
		score -= 6
	}
	if trace.Insights.RetryCount > 0 {
		score -= float64(trace.Insights.RetryCount * 2)
	}
	return clampScore(score)
}

func controlScore(trace models.Trace) float64 {
	coverage := scorePolicyCoverage(trace).Score
	score := coverage
	if trace.Insights.LLMCalls > 0 && len(trace.PolicyEvents) == 0 {
		score -= 20
	}
	if trace.Insights.BlockedSpans > 0 && trace.Status != "error" {
		score += 5
	}
	return clampScore(score)
}

func boundaryScore(trace models.Trace, tags map[string]struct{}) float64 {
	if hasTag(tags, "red-team") || hasTag(tags, "security") {
		return securityScore(trace, tags)
	}
	score := 100.0
	score -= float64(trace.Insights.BlockedSpans * 8)
	score -= float64(trace.Insights.RedactedSpans * 4)
	score -= float64(trace.ErrorCount * 10)
	score = (score * 0.5) + (controlScore(trace) * 0.5)
	return clampScore(score)
}

func securityScore(trace models.Trace, tags map[string]struct{}) float64 {
	summary := summarizePolicyEffectiveness(trace)
	if hasTag(tags, "red-team") {
		score := 25.0
		if summary.Denies > 0 {
			score += 35
		}
		if summary.Redacts > 0 {
			score += 20
		}
		if trace.Insights.BlockedSpans > 0 {
			score += 20
		}
		if summary.Warns > 0 {
			score += 5
		}
		if summary.TotalEvents == 0 {
			score -= 10
		}
		return clampScore(score)
	}
	score := controlScore(trace)
	if trace.Insights.BlockedSpans > 0 || summary.Denies > 0 {
		score -= 10
	}
	return clampScore(score)
}

func evidenceScore(trace models.Trace) float64 {
	score := 35.0
	if len(trace.Spans) > 0 {
		score += 15
	}
	if trace.Timeline != nil && len(trace.Timeline.Items) > 0 {
		score += 15
	}
	if len(trace.PolicyEvents) > 0 {
		score += 15
	}
	if trace.TotalTokens > 0 {
		score += 10
	}
	for _, span := range trace.Spans {
		if strings.TrimSpace(span.PromptID) != "" || strings.TrimSpace(span.PromptPreview) != "" {
			score += 10
			break
		}
	}
	return clampScore(score)
}

func riskFromPackThresholds(pack EvalPackDefinition, overall float64) string {
	passing := 85.0
	warning := 70.0
	if value, ok := numberFromMap(pack.Defaults, "passing_threshold"); ok {
		passing = normalizeThreshold(value) * 100
	}
	if value, ok := numberFromMap(pack.Defaults, "warning_threshold"); ok {
		warning = normalizeThreshold(value) * 100
	}
	if overall >= passing {
		return "low"
	}
	if overall >= warning {
		return "medium"
	}
	return "high"
}

func normalizeThreshold(value float64) float64 {
	if value > 1 {
		return value / 100.0
	}
	return value
}

func nestedMapValue(root map[string]any, keys ...string) any {
	var current any = root
	for _, key := range keys {
		child, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = child[key]
	}
	return current
}

func nestedNumber(root map[string]any, keys ...string) (float64, bool) {
	value := nestedMapValue(root, keys...)
	return numberFromAny(value)
}

func numberFromMap(root map[string]any, key string) (float64, bool) {
	return numberFromAny(root[key])
}

func numberFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func normalizeTags(tags []string) map[string]struct{} {
	out := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		out[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	return out
}

func hasTag(tags map[string]struct{}, tag string) bool {
	_, ok := tags[strings.ToLower(strings.TrimSpace(tag))]
	return ok
}

func maxEvalCount(left, right int) int {
	if left > right {
		return left
	}
	return right
}
