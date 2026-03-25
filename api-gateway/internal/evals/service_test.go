package evals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentfabric/api-gateway/internal/models"
	"github.com/agentfabric/api-gateway/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestPolicyEventsFromAudit(t *testing.T) {
	spans := []models.Span{
		{
			ID:       "span-1",
			Model:    "gpt-4o",
			Provider: "openai",
			Attributes: map[string]string{
				"af.policy.scope":      "request",
				"af.policy.matched":    "app,environment",
				"af.policy.redactions": "2",
			},
		},
	}
	entries := []store.AuditEntry{
		{
			DecisionID: "decision-1",
			TraceID:    "trace-1",
			SpanID:     "span-1",
			PolicyName: "deny-prod",
			Result:     "deny",
			Reason:     "not allowed",
			TenantID:   "tenant-1",
		},
	}

	got := policyEventsFromAudit(entries, spans)
	if len(got) != 1 {
		t.Fatalf("expected 1 policy event, got %d", len(got))
	}
	if got[0].Provider != "openai" || got[0].Model != "gpt-4o" {
		t.Fatalf("unexpected policy event enrichment: %#v", got[0])
	}
	if got[0].Redactions != 2 || len(got[0].Matched) != 2 {
		t.Fatalf("unexpected policy event details: %#v", got[0])
	}
}

type fakeEvalStore struct {
	inputsByTrace map[string]*store.TraceViewInputs
	insertedRun   models.TraceEvalRun
	insertErr     error
	listRuns      []models.TraceEvalRun
	listRunsErr   error
	releaseRuns   map[string][]models.TraceEvalRun
	releaseErr    error
}

func (f *fakeEvalStore) LoadTraceViewInputs(_ context.Context, traceID, _ string) (*store.TraceViewInputs, error) {
	if inputs, ok := f.inputsByTrace[traceID]; ok {
		return inputs, nil
	}
	return &store.TraceViewInputs{}, nil
}

func (f *fakeEvalStore) InsertEvalRun(_ context.Context, _ string, run models.TraceEvalRun) (models.TraceEvalRun, error) {
	if f.insertErr != nil {
		return models.TraceEvalRun{}, f.insertErr
	}
	run.ID = 99
	run.CreatedAt = time.Unix(0, 0).UTC()
	f.insertedRun = run
	return run, nil
}

func (f *fakeEvalStore) ListEvalRuns(_ context.Context, _ string, _ int) ([]models.TraceEvalRun, error) {
	return f.listRuns, f.listRunsErr
}

func (f *fakeEvalStore) ListEvalRunsByRelease(_ context.Context, _ string, promptID, promptEnvironment, releaseTag, _ string) ([]models.TraceEvalRun, error) {
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	key := releaseTag
	if promptID != "" || promptEnvironment != "" {
		key = promptID + "::" + promptEnvironment + "::" + releaseTag
	}
	return f.releaseRuns[key], nil
}

func TestScoreTraceValidatesTraceID(t *testing.T) {
	svc := &Service{store: &fakeEvalStore{}}
	if _, err := svc.ScoreTrace(context.Background(), "tenant-1", models.TraceEvalRequest{}); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestScoreTraceBuildsAndPersistsEvalRun(t *testing.T) {
	fake := &fakeEvalStore{
		inputsByTrace: map[string]*store.TraceViewInputs{
			"trace-1": {
				Spans: []models.Span{
					{ID: "root", TraceID: "trace-1", RunID: "run-1", Name: "root", Framework: "langgraph", DurationNs: 1_200_000_000, Attributes: map[string]string{"af.step_type": "llm", "af.prompt.id": "support.system", "af.prompt.version": "4", "af.prompt.environment": "staging", "af.prompt.release_tag": "candidate-1"}, ReceivedAt: time.Now().UTC()},
				},
				AuditEntries: []store.AuditEntry{
					{DecisionID: "d1", TraceID: "trace-1", SpanID: "root", PolicyName: "allow", Result: "allow", Reason: "ok", TenantID: "tenant-1"},
				},
			},
		},
	}
	svc := &Service{store: fake}

	run, err := svc.ScoreTrace(context.Background(), "tenant-1", models.TraceEvalRequest{
		TraceID:    "trace-1",
		ReleaseTag: "candidate-1",
	})
	if err != nil {
		t.Fatalf("ScoreTrace returned error: %v", err)
	}
	if run.ID != 99 || run.ReleaseTag != "candidate-1" {
		t.Fatalf("unexpected run: %#v", run)
	}
	if run.PromptID != "support.system" || run.PromptVersion != 4 || run.PromptEnvironment != "staging" {
		t.Fatalf("expected prompt lineage to be inferred, got %#v", run)
	}
	if len(run.Scores) != 4 {
		t.Fatalf("expected 4 scores, got %d", len(run.Scores))
	}
	if fake.insertedRun.TraceID != "trace-1" {
		t.Fatalf("expected inserted run to use trace-1, got %#v", fake.insertedRun)
	}
}

func TestScoreTraceReturnsNoRowsForEmptyTrace(t *testing.T) {
	fake := &fakeEvalStore{
		inputsByTrace: map[string]*store.TraceViewInputs{
			"trace-empty": {Spans: nil},
		},
	}
	svc := &Service{store: fake}
	_, err := svc.ScoreTrace(context.Background(), "tenant-1", models.TraceEvalRequest{TraceID: "trace-empty"})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestListRunsDelegatesToStore(t *testing.T) {
	expected := []models.TraceEvalRun{{ID: 1}, {ID: 2}}
	svc := &Service{store: &fakeEvalStore{listRuns: expected}}
	got, err := svc.ListRuns(context.Background(), "tenant-1", 5)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(got))
	}
}

func TestCompareReleaseBuildsRegressionReport(t *testing.T) {
	fake := &fakeEvalStore{
		releaseRuns: map[string][]models.TraceEvalRun{
			"prompt-a::staging::baseline": {
				{Scores: []models.TraceEvalScore{{Metric: "latency", Score: 80}, {Metric: "reliability", Score: 90}}},
			},
			"prompt-a::staging::candidate": {
				{Scores: []models.TraceEvalScore{{Metric: "latency", Score: 60}, {Metric: "reliability", Score: 70}}},
			},
		},
	}
	svc := &Service{store: fake}
	report, err := svc.CompareRelease(context.Background(), "tenant-1", models.RegressionCompareRequest{
		BaselineTag:  "baseline",
		CandidateTag: "candidate",
		PromptID:     "prompt-a",
		Environment:  "staging",
	})
	if err != nil {
		t.Fatalf("CompareRelease returned error: %v", err)
	}
	if report.RiskLevel == "" || len(report.Metrics) == 0 {
		t.Fatalf("unexpected regression report: %#v", report)
	}
}
