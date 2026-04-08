package store

import (
	"context"

	"github.com/govagn/api-gateway/internal/models"
)

type TraceViewInputs struct {
	Spans        []models.Span
	AuditEntries []AuditEntry
}

func (s *PostgresStore) LoadTraceViewInputs(ctx context.Context, traceID, tenantID string) (*TraceViewInputs, error) {
	spans, err := s.GetTraceSpans(ctx, traceID, tenantID)
	if err != nil {
		return nil, err
	}
	entries, err := s.ListAuditEntriesForTrace(ctx, tenantID, traceID)
	if err != nil {
		return nil, err
	}
	return &TraceViewInputs{
		Spans:        spans,
		AuditEntries: entries,
	}, nil
}
