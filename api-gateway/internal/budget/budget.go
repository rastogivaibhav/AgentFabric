package budget

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Budget represents a tenant's monthly quota
type Budget struct {
	TenantID       string
	MonthlyTokens  int64
	MonthlyCostUSD float64
	AlertThreshold float64 // 0.80 = alert at 80%
	HardLimit      bool    // true = block at 100%, false = alert only
	ResetDay       int     // day of month to reset
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UsageSummary represents current period usage
type UsageSummary struct {
	TokensUsed  int64
	CostUsedUSD float64
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// Store interface for budget queries
type Store interface {
	GetBudget(ctx context.Context, tenantID string) (*Budget, error)
	GetMonthlyUsage(ctx context.Context, tenantID string, since time.Time) (*UsageSummary, error)
	UpsertBudget(ctx context.Context, b *Budget) error
	RecordAlert(ctx context.Context, tenantID, alertType string, usage UsageSummary, b Budget) error
	ListBudgetAlerts(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error)
}

// Alerter sends notifications when budgets are hit
type Alerter interface {
	Fire(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error
}

// BudgetEnforcer checks and records token/cost usage against budgets
type BudgetEnforcer struct {
	store      Store
	alerter    Alerter
	logger     *zap.Logger
	mu         sync.RWMutex
	alertedAt  map[string]time.Time // dedup: tenant+alertType -> last alert time
}

// NewBudgetEnforcer creates a new enforcer
func NewBudgetEnforcer(store Store, alerter Alerter, logger *zap.Logger) *BudgetEnforcer {
	return &BudgetEnforcer{
		store:     store,
		alerter:   alerter,
		logger:    logger,
		alertedAt: make(map[string]time.Time),
	}
}

// CheckAndRecord checks usage against budget and records the usage
// Returns (allowed bool, err error)
// If allowed=false, the ingest should return HTTP 429
// If err != nil, we failed to check but don't block (fail open)
func (e *BudgetEnforcer) CheckAndRecord(
	ctx context.Context,
	tenantID string,
	incomingTokens int64,
	incomingCostUSD float64,
) (bool, error) {
	if tenantID == "" {
		return true, nil // no budget for empty tenant
	}

	budget, err := e.store.GetBudget(ctx, tenantID)
	if err != nil {
		// Budget lookup failed — fail open (don't block), log error
		e.logger.Warn("failed to get budget", zap.String("tenant", tenantID), zap.Error(err))
		return true, nil
	}

	// No budget set = unlimited
	if budget == nil || (budget.MonthlyTokens == 0 && budget.MonthlyCostUSD == 0) {
		return true, nil
	}

	// Get current period start based on reset_day
	periodStart := e.currentPeriodStart(budget.ResetDay)
	usage, err := e.store.GetMonthlyUsage(ctx, tenantID, periodStart)
	if err != nil {
		e.logger.Warn("failed to get usage", zap.String("tenant", tenantID), zap.Error(err))
		return true, nil
	}

	// Check token limit
	if budget.MonthlyTokens > 0 {
		projected := usage.TokensUsed + incomingTokens
		percentUsed := float64(projected) / float64(budget.MonthlyTokens)

		if projected > budget.MonthlyTokens && budget.HardLimit {
			// Hard limit hit — block and alert
			e.fireAlert(ctx, tenantID, "hard_limit", *usage, *budget)
			return false, nil
		}

		if percentUsed > budget.AlertThreshold {
			// Threshold hit — alert but allow
			e.fireAlert(ctx, tenantID, "threshold_80", *usage, *budget)
		}
	}

	// Check cost limit
	if budget.MonthlyCostUSD > 0 {
		projected := usage.CostUsedUSD + incomingCostUSD
		percentUsed := projected / budget.MonthlyCostUSD

		if projected > budget.MonthlyCostUSD && budget.HardLimit {
			// Hard limit hit — block and alert
			e.fireAlert(ctx, tenantID, "cost_hard_limit", *usage, *budget)
			return false, nil
		}

		if percentUsed > budget.AlertThreshold {
			// Threshold hit — alert but allow
			e.fireAlert(ctx, tenantID, "cost_threshold_80", *usage, *budget)
		}
	}

	return true, nil
}

// fireAlert fires an alert, with deduplication to avoid flooding
func (e *BudgetEnforcer) fireAlert(
	ctx context.Context,
	tenantID string,
	alertType string,
	usage UsageSummary,
	budget Budget,
) {
	key := tenantID + ":" + alertType
	now := time.Now()

	e.mu.Lock()
	lastAlert, exists := e.alertedAt[key]
	e.mu.Unlock()

	// Dedupe: only fire once per hour
	if exists && now.Sub(lastAlert) < time.Hour {
		return
	}

	go func() {
		if err := e.alerter.Fire(ctx, tenantID, alertType, usage, budget); err != nil {
			e.logger.Warn("alert fire failed", zap.String("tenant", tenantID), zap.String("type", alertType), zap.Error(err))
			return
		}

		// Record in DB
		if err := e.store.RecordAlert(ctx, tenantID, alertType, usage, budget); err != nil {
			e.logger.Warn("failed to record alert", zap.String("tenant", tenantID), zap.Error(err))
		}

		e.mu.Lock()
		e.alertedAt[key] = now
		e.mu.Unlock()
	}()
}

// currentPeriodStart returns the start of the current billing period
func (e *BudgetEnforcer) currentPeriodStart(resetDay int) time.Time {
	now := time.Now().UTC()
	year, month, day := now.Date()

	if day >= resetDay {
		// Current period started on resetDay of this month
		return time.Date(year, month, resetDay, 0, 0, 0, 0, time.UTC)
	}

	// Current period started on resetDay of previous month
	prevMonth := month - 1
	prevYear := year
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}
	return time.Date(prevYear, time.Month(prevMonth), resetDay, 0, 0, 0, 0, time.UTC)
}

// NoopAlerter is a no-op alerter for testing
type NoopAlerter struct{}

func (n *NoopAlerter) Fire(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error {
	return nil
}

// WebhookAlerter sends alerts via HTTP webhook (stub for now)
type WebhookAlerter struct {
	logger *zap.Logger
}

func NewWebhookAlerter(logger *zap.Logger) *WebhookAlerter {
	return &WebhookAlerter{logger: logger}
}

func (w *WebhookAlerter) Fire(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error {
	// TODO: Implement webhook dispatch
	w.logger.Debug("alert fired", zap.String("tenant", tenantID), zap.String("type", alertType))
	return nil
}
