package budget

import (
	"context"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestBudgetRequestLifecycle(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()
	repo := NewPostgresRepository(pool)

	tenantID := uuid.New()
	reviewerID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name) VALUES ($1, 'budget-tenant', 'Budget Tenant')`,
		tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, 'reviewer@test.local', 'x')`,
		reviewerID); err != nil {
		t.Fatalf("seed reviewer: %v", err)
	}

	req := &domain.BudgetRequest{
		TenantID: tenantID, RequestedAmount: decimal.NewFromInt(1000),
		Reason: "扩容", Status: domain.BudgetRequestPending,
	}
	if err := repo.CreateRequest(ctx, req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	requests, err := repo.ListRequestsByTenant(ctx, tenantID)
	if err != nil || len(requests) != 1 || requests[0].Status != domain.BudgetRequestPending {
		t.Fatalf("list requests = %+v err=%v", requests, err)
	}

	budget, err := repo.ApproveRequest(ctx, req.ID, reviewerID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !budget.LimitAmount.Equal(decimal.NewFromInt(1000)) {
		t.Errorf("limit = %s, want 1000", budget.LimitAmount)
	}

	// A second approved request adds to the same monthly budget.
	req2 := &domain.BudgetRequest{
		TenantID: tenantID, RequestedAmount: decimal.NewFromInt(500),
		Reason: "再扩容", Status: domain.BudgetRequestPending,
	}
	if err := repo.CreateRequest(ctx, req2); err != nil {
		t.Fatalf("create request 2: %v", err)
	}
	budget2, err := repo.ApproveRequest(ctx, req2.ID, reviewerID)
	if err != nil {
		t.Fatalf("approve 2: %v", err)
	}
	if !budget2.LimitAmount.Equal(decimal.NewFromInt(1500)) {
		t.Errorf("limit = %s, want 1500", budget2.LimitAmount)
	}

	// Reject a third request.
	req3 := &domain.BudgetRequest{
		TenantID: tenantID, RequestedAmount: decimal.NewFromInt(999),
		Reason: "应拒绝", Status: domain.BudgetRequestPending,
	}
	if err := repo.CreateRequest(ctx, req3); err != nil {
		t.Fatalf("create request 3: %v", err)
	}
	if err := repo.RejectRequest(ctx, req3.ID, reviewerID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	all, _ := repo.ListRequestsByTenant(ctx, tenantID)
	statuses := map[domain.BudgetRequestStatus]int{}
	for _, r := range all {
		statuses[r.Status]++
	}
	if statuses[domain.BudgetRequestApproved] != 2 || statuses[domain.BudgetRequestRejected] != 1 {
		t.Errorf("statuses = %+v", statuses)
	}
}
