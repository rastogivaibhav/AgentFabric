package budget

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"go.uber.org/zap"
)

// MockStore implements Store for testing
type MockStore struct {
	budgets map[string]*Budget
	usage   map[string]UsageSummary
	alerts  []map[string]interface{}
}

func NewMockStore() *MockStore {
	return &MockStore{
		budgets: make(map[string]*Budget),
		usage:   make(map[string]UsageSummary),
		alerts:  make([]map[string]interface{}, 0),
	}
}

func (m *MockStore) GetBudget(ctx context.Context, tenantID string) (*Budget, error) {
	if b, ok := m.budgets[tenantID]; ok {
		return b, nil
	}
	return nil, sql.ErrNoRows
}

func (m *MockStore) GetMonthlyUsage(ctx context.Context, tenantID string, since time.Time) (*UsageSummary, error) {
	if u, ok := m.usage[tenantID]; ok {
		return &u, nil
	}
	return &UsageSummary{}, nil
}

func (m *MockStore) UpsertBudget(ctx context.Context, b *Budget) error {
	m.budgets[b.TenantID] = b
	return nil
}

func (m *MockStore) ListBudgetAlerts(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error) {
	var out []map[string]interface{}
	for _, a := range m.alerts {
		if a["tenantID"] == tenantID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockStore) RecordAlert(ctx context.Context, tenantID, alertType string, usage UsageSummary, budget Budget) error {
	m.alerts = append(m.alerts, map[string]interface{}{
		"tenantID":  tenantID,
		"alertType": alertType,
		"usage":     usage,
		"budget":    budget,
	})
	return nil
}

func TestBudgetEnforcer_Unlimited(t *testing.T) {
	store := NewMockStore()
	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// No budget set = unlimited
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 1000, 100.0)
	if !allowed || err != nil {
		t.Fatalf("expected allowed=true, got %v, err=%v", allowed, err)
	}
}

func TestBudgetEnforcer_UnderLimit(t *testing.T) {
	store := NewMockStore()
	store.budgets["tenant1"] = &Budget{
		TenantID:       "tenant1",
		MonthlyTokens:  10000,
		MonthlyCostUSD: 1000.0,
		AlertThreshold: 0.80,
		HardLimit:      true,
		ResetDay:       1,
	}
	store.usage["tenant1"] = UsageSummary{
		TokensUsed:  5000,
		CostUsedUSD: 500.0,
	}

	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 100, 10.0)
	if !allowed || err != nil {
		t.Fatalf("expected allowed=true, got %v, err=%v", allowed, err)
	}
}

func TestBudgetEnforcer_OverTokenLimit(t *testing.T) {
	store := NewMockStore()
	store.budgets["tenant1"] = &Budget{
		TenantID:       "tenant1",
		MonthlyTokens:  10000,
		MonthlyCostUSD: 0,
		AlertThreshold: 0.80,
		HardLimit:      true,
		ResetDay:       1,
	}
	store.usage["tenant1"] = UsageSummary{
		TokensUsed:  9500,
		CostUsedUSD: 0,
	}

	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// 9500 + 1000 = 10500 > 10000 (hard limit)
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 1000, 0)
	if allowed || err != nil {
		t.Fatalf("expected allowed=false, got %v, err=%v", allowed, err)
	}
}

func TestBudgetEnforcer_OverCostLimit(t *testing.T) {
	store := NewMockStore()
	store.budgets["tenant1"] = &Budget{
		TenantID:       "tenant1",
		MonthlyTokens:  0,
		MonthlyCostUSD: 1000.0,
		AlertThreshold: 0.80,
		HardLimit:      true,
		ResetDay:       1,
	}
	store.usage["tenant1"] = UsageSummary{
		TokensUsed:  0,
		CostUsedUSD: 950.0,
	}

	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// 950 + 100 = 1050 > 1000 (hard limit)
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 0, 100.0)
	if allowed || err != nil {
		t.Fatalf("expected allowed=false, got %v, err=%v", allowed, err)
	}
}

func TestBudgetEnforcer_SoftLimitOnly(t *testing.T) {
	store := NewMockStore()
	store.budgets["tenant1"] = &Budget{
		TenantID:       "tenant1",
		MonthlyTokens:  10000,
		MonthlyCostUSD: 0,
		AlertThreshold: 0.80,
		HardLimit:      false, // soft limit
		ResetDay:       1,
	}
	store.usage["tenant1"] = UsageSummary{
		TokensUsed:  9500,
		CostUsedUSD: 0,
	}

	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// 9500 + 1000 = 10500 > 10000, but soft limit = should be allowed
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 1000, 0)
	if !allowed || err != nil {
		t.Fatalf("expected allowed=true (soft limit), got %v, err=%v", allowed, err)
	}
}

func TestBudgetEnforcer_AlertThreshold(t *testing.T) {
	store := NewMockStore()
	store.budgets["tenant1"] = &Budget{
		TenantID:       "tenant1",
		MonthlyTokens:  10000,
		MonthlyCostUSD: 0,
		AlertThreshold: 0.80,
		HardLimit:      true,
		ResetDay:       1,
	}
	store.usage["tenant1"] = UsageSummary{
		TokensUsed:  8000,
		CostUsedUSD: 0,
	}

	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// 8000 + 100 = 8100 (81% of 10000) > 80% threshold
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 100, 0)
	if !allowed || err != nil {
		t.Fatalf("expected allowed=true (alert threshold), got %v, err=%v", allowed, err)
	}
	// Should have triggered an alert (in background)
	time.Sleep(100 * time.Millisecond)
	if len(store.alerts) == 0 {
		t.Fatalf("expected alert to be recorded, got %d", len(store.alerts))
	}
}

func TestBudgetEnforcer_NoBudget(t *testing.T) {
	store := NewMockStore()
	alerter := &NoopAlerter{}
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(store, alerter, logger)

	// No budget stored = unlimited
	allowed, err := enforcer.CheckAndRecord(context.Background(), "tenant1", 1000000, 999999.0)
	if !allowed || err != nil {
		t.Fatalf("expected allowed=true, got %v, err=%v", allowed, err)
	}
}

func TestCurrentPeriodStart(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	enforcer := NewBudgetEnforcer(NewMockStore(), &NoopAlerter{}, logger)

	tests := []struct {
		resetDay  int
		name      string
		checkFn   func(t *testing.T, start time.Time)
	}{
		{
			resetDay: 1,
			name:     "first of month",
			checkFn: func(t *testing.T, start time.Time) {
				if start.Day() != 1 {
					t.Fatalf("expected day 1, got %d", start.Day())
				}
			},
		},
		{
			resetDay: 15,
			name:     "15th of month",
			checkFn: func(t *testing.T, start time.Time) {
				if start.Day() != 15 {
					t.Fatalf("expected day 15, got %d", start.Day())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := enforcer.currentPeriodStart(tt.resetDay)
			if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
				t.Fatalf("expected midnight, got %v", start)
			}
			tt.checkFn(t, start)
		})
	}
}
