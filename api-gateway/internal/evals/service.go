package evals

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/observability"
	"github.com/govagn/api-gateway/internal/store"
	"github.com/jackc/pgx/v5"
)

const defaultSuite = "core-release"

type Service struct {
	store       evalStore
	packRoot    string
	datasetRoot string
}

type evalStore interface {
	LoadTraceViewInputs(ctx context.Context, traceID, tenantID string) (*store.TraceViewInputs, error)
	InsertEvalRun(ctx context.Context, tenantID string, run models.TraceEvalRun) (models.TraceEvalRun, error)
	ListEvalRuns(ctx context.Context, tenantID string, limit int) ([]models.TraceEvalRun, error)
	ListEvalRunsByRelease(ctx context.Context, tenantID, promptID, promptEnvironment, releaseTag, evalSuite string) ([]models.TraceEvalRun, error)
	ListAgentScorecardMetrics(ctx context.Context, tenantID string, windowStart, windowEnd time.Time, limit int, agentName string) ([]models.AgentScorecardMetrics, error)
	ListEvalDatasets(ctx context.Context, tenantID string, limit int) ([]models.EvalDataset, error)
	UpsertEvalDataset(ctx context.Context, tenantID string, dataset models.EvalDataset) (models.EvalDataset, error)
	ListEvalDatasetItems(ctx context.Context, tenantID string, datasetRefs []string, limit int) ([]models.EvalDatasetItem, error)
	CreateEvalExecution(ctx context.Context, tenantID string, execution models.EvalExecution) (models.EvalExecution, error)
	UpdateEvalExecution(ctx context.Context, tenantID string, execution models.EvalExecution) (models.EvalExecution, error)
	InsertEvalExecutionItem(ctx context.Context, tenantID string, item models.EvalExecutionItem) (models.EvalExecutionItem, error)
	InsertEvalEvaluatorResults(ctx context.Context, tenantID string, executionID, itemID int64, results []models.EvalEvaluatorResult) error
	InsertEvalEvidenceLinks(ctx context.Context, tenantID string, executionID, itemID int64, links []models.EvalEvidenceLink) error
	ListEvalExecutions(ctx context.Context, tenantID string, limit int) ([]models.EvalExecution, error)
	GetEvalExecution(ctx context.Context, tenantID string, executionID int64) (models.EvalExecution, error)
	DeleteEvalExecutionDetails(ctx context.Context, tenantID string, executionID int64) error
}

func NewService(pg *store.PostgresStore) *Service {
	return &Service{store: pg, packRoot: defaultPackRoot(), datasetRoot: defaultDatasetRoot()}
}

func NewServiceWithPackRoot(pg *store.PostgresStore, packRoot string) *Service {
	return NewServiceWithRoots(pg, packRoot, "")
}

func NewServiceWithRoots(pg *store.PostgresStore, packRoot, datasetRoot string) *Service {
	root := strings.TrimSpace(packRoot)
	if root == "" {
		root = defaultPackRoot()
	}
	dsRoot := strings.TrimSpace(datasetRoot)
	if dsRoot == "" {
		dsRoot = defaultDatasetRoot()
	}
	return &Service{store: pg, packRoot: root, datasetRoot: dsRoot}
}

func (s *Service) ScoreTrace(ctx context.Context, tenantID string, req models.TraceEvalRequest) (models.TraceEvalRun, error) {
	traceIDs := normalizedTraceIDs(req)
	if len(traceIDs) == 0 {
		return models.TraceEvalRun{}, fmt.Errorf("trace_id is required")
	}
	suite := strings.TrimSpace(req.EvalSuite)
	if suite == "" {
		suite = strings.TrimSpace(req.PackID)
	}
	if suite == "" {
		suite = defaultSuite
	}
	if _, err := GetPackDefinition(s.packRoot, suite); err == nil {
		execution, run, err := s.ExecutePack(ctx, tenantID, models.TraceEvalRequest{
			TraceID:     strings.TrimSpace(req.TraceID),
			TraceIDs:    traceIDs,
			ReleaseTag:  strings.TrimSpace(req.ReleaseTag),
			EvalSuite:   suite,
			PackID:      suite,
			Mode:        firstNonEmpty(strings.TrimSpace(req.Mode), "offline"),
			DatasetRefs: req.DatasetRefs,
			Attributes:  req.Attributes,
			SampleLimit: req.SampleLimit,
		})
		if err != nil {
			return models.TraceEvalRun{}, err
		}
		if len(traceIDs) == 1 {
			run.TraceID = traceIDs[0]
		}
		if run.ReleaseTag == "" {
			run.ReleaseTag = execution.ReleaseTag
		}
		return run, nil
	}

	trace, lineage, err := s.loadTraceEvidence(ctx, tenantID, traceIDs[0])
	if err != nil {
		return models.TraceEvalRun{}, err
	}
	scores, overall, risk, summary := s.scoreTraceSuite(suite, trace)
	run := models.TraceEvalRun{
		TraceID:             trace.ID,
		PromptID:            lineage.PromptID,
		PromptVersion:       lineage.PromptVersion,
		PromptEnvironment:   lineage.PromptEnvironment,
		ReleaseTag:          firstNonEmpty(strings.TrimSpace(req.ReleaseTag), lineage.ReleaseTag),
		EvalSuite:           suite,
		OverallScore:        overall,
		RiskLevel:           risk,
		Summary:             summary,
		PolicyEffectiveness: summarizePolicyEffectiveness(trace),
		Scores:              scores,
	}
	return s.store.InsertEvalRun(ctx, tenantID, run)
}

func (s *Service) ListRuns(ctx context.Context, tenantID string, limit int) ([]models.TraceEvalRun, error) {
	return s.store.ListEvalRuns(ctx, tenantID, limit)
}

func (s *Service) ListDatasets(ctx context.Context, tenantID string, limit int) ([]models.EvalDataset, error) {
	dbItems, err := s.store.ListEvalDatasets(ctx, tenantID, limit)
	if err != nil {
		return nil, err
	}
	seedItems, err := LoadDatasetDefinitions(s.datasetRoot)
	if err != nil {
		return dbItems, nil
	}
	return append(seedItems, dbItems...), nil
}

func (s *Service) UpsertDataset(ctx context.Context, tenantID string, dataset models.EvalDataset) (models.EvalDataset, error) {
	return s.store.UpsertEvalDataset(ctx, tenantID, dataset)
}

func (s *Service) ListExecutions(ctx context.Context, tenantID string, limit int) ([]models.EvalExecution, error) {
	return s.store.ListEvalExecutions(ctx, tenantID, limit)
}

func (s *Service) GetExecution(ctx context.Context, tenantID string, executionID int64) (models.EvalExecution, error) {
	return s.store.GetEvalExecution(ctx, tenantID, executionID)
}

func (s *Service) scoreTraceSuite(suite string, trace models.Trace) ([]models.TraceEvalScore, float64, string, string) {
	if scores, overall, risk, summary, ok := scoreTraceWithPack(s.packRoot, suite, trace); ok {
		return scores, overall, risk, summary
	}

	scores := make([]models.TraceEvalScore, 0, len(defaultScorers()))
	var weightedTotal float64
	var weightTotal float64
	for _, scorer := range defaultScorers() {
		score := scorer.run(trace)
		score.Weight = scorer.weight
		scores = append(scores, score)
		weightedTotal += score.Score * scorer.weight
		weightTotal += scorer.weight
	}
	overall := 0.0
	if weightTotal > 0 {
		overall = clampScore(weightedTotal / weightTotal)
	}
	risk := severityForScore(overall)
	summary := fmt.Sprintf("Trace %s scored %.2f across %d metrics", trace.ID, overall, len(scores))
	return scores, overall, risk, summary
}

func (s *Service) loadTraceEvidence(ctx context.Context, tenantID, traceID string) (models.Trace, promptLineage, error) {
	inputs, err := s.store.LoadTraceViewInputs(ctx, traceID, tenantID)
	if err != nil {
		return models.Trace{}, promptLineage{}, err
	}
	if len(inputs.Spans) == 0 {
		return models.Trace{}, promptLineage{}, pgx.ErrNoRows
	}
	baseSpans := observability.EnrichSpans(inputs.Spans, nil)
	policyEvents := policyEventsFromAudit(inputs.AuditEntries, baseSpans)
	trace := observability.BuildTrace(traceID, inputs.Spans, policyEvents)
	trace.Timeline = observability.BuildTimeline(traceID, trace.Spans, policyEvents)
	return trace, derivePromptLineage(trace.Spans), nil
}

func normalizedTraceIDs(req models.TraceEvalRequest) []string {
	ids := make([]string, 0, len(req.TraceIDs)+1)
	seen := map[string]struct{}{}
	if trimmed := strings.TrimSpace(req.TraceID); trimmed != "" {
		seen[trimmed] = struct{}{}
		ids = append(ids, trimmed)
	}
	for _, id := range req.TraceIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			ids = append(ids, trimmed)
		}
	}
	return ids
}

func policyEventsFromAudit(entries []store.AuditEntry, spans []models.Span) []models.PolicyEvent {
	if len(entries) == 0 {
		return nil
	}
	spanByID := make(map[string]models.Span, len(spans))
	for _, span := range spans {
		spanByID[span.ID] = span
	}
	events := make([]models.PolicyEvent, 0, len(entries))
	for _, entry := range entries {
		event := models.PolicyEvent{
			DecisionID: entry.DecisionID,
			TraceID:    entry.TraceID,
			SpanID:     entry.SpanID,
			PolicyName: entry.PolicyName,
			Result:     entry.Result,
			Reason:     entry.Reason,
			TenantID:   entry.TenantID,
		}
		if span, ok := spanByID[entry.SpanID]; ok {
			event.Provider = span.Provider
			event.Model = span.Model
			event.Scope = firstNonEmpty(span.Attributes["af.policy.scope"], span.Attributes["policy.scope"])
			if matched := firstNonEmpty(span.Attributes["af.policy.matched"], span.Attributes["policy.matched"]); matched != "" {
				event.Matched = splitCSV(matched)
			}
			if redactions := firstInt(span.Attributes, "af.policy.redactions", "policy.redactions"); redactions > 0 {
				event.Redactions = redactions
			}
		}
		events = append(events, event)
	}
	return events
}

type promptLineage struct {
	PromptID          string
	PromptVersion     int
	PromptEnvironment string
	ReleaseTag        string
}

func derivePromptLineage(spans []models.Span) promptLineage {
	var lineage promptLineage
	for _, span := range spans {
		if lineage.PromptID == "" && strings.TrimSpace(span.PromptID) != "" {
			lineage.PromptID = strings.TrimSpace(span.PromptID)
		}
		if lineage.PromptVersion == 0 && span.PromptVersion > 0 {
			lineage.PromptVersion = span.PromptVersion
		}
		if lineage.PromptEnvironment == "" && strings.TrimSpace(span.PromptEnvironment) != "" {
			lineage.PromptEnvironment = strings.TrimSpace(span.PromptEnvironment)
		}
		if lineage.ReleaseTag == "" && strings.TrimSpace(span.PromptReleaseTag) != "" {
			lineage.ReleaseTag = strings.TrimSpace(span.PromptReleaseTag)
		}
		if lineage.PromptID != "" && lineage.PromptVersion > 0 && lineage.PromptEnvironment != "" && lineage.ReleaseTag != "" {
			break
		}
	}
	return lineage
}
