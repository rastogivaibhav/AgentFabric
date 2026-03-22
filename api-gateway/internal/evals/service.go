package evals

import (
	"context"
	"fmt"
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/observability"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/jackc/pgx/v5"
)

const defaultSuite = "core-release"

type Service struct {
	store *store.PostgresStore
}

func NewService(pg *store.PostgresStore) *Service {
	return &Service{store: pg}
}

func (s *Service) ScoreTrace(ctx context.Context, tenantID string, req models.TraceEvalRequest) (models.TraceEvalRun, error) {
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		return models.TraceEvalRun{}, fmt.Errorf("trace_id is required")
	}
	inputs, err := s.store.LoadTraceViewInputs(ctx, traceID, tenantID)
	if err != nil {
		return models.TraceEvalRun{}, err
	}
	if len(inputs.Spans) == 0 {
		return models.TraceEvalRun{}, pgx.ErrNoRows
	}
	baseSpans := observability.EnrichSpans(inputs.Spans, nil)
	policyEvents := policyEventsFromAudit(inputs.AuditEntries, baseSpans)
	trace := observability.BuildTrace(traceID, inputs.Spans, policyEvents)
	trace.Timeline = observability.BuildTimeline(traceID, trace.Spans, policyEvents)

	suite := strings.TrimSpace(req.EvalSuite)
	if suite == "" {
		suite = defaultSuite
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
	run := models.TraceEvalRun{
		TraceID:             traceID,
		ReleaseTag:          strings.TrimSpace(req.ReleaseTag),
		EvalSuite:           suite,
		OverallScore:        overall,
		RiskLevel:           risk,
		Summary:             fmt.Sprintf("Trace %s scored %.2f across %d metrics", traceID, overall, len(scores)),
		PolicyEffectiveness: summarizePolicyEffectiveness(trace),
		Scores:              scores,
	}
	return s.store.InsertEvalRun(ctx, tenantID, run)
}

func (s *Service) ListRuns(ctx context.Context, tenantID string, limit int) ([]models.TraceEvalRun, error) {
	return s.store.ListEvalRuns(ctx, tenantID, limit)
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
