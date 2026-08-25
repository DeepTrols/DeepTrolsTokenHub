package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type budgetResponse struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name"`
	Period      string `json:"period"`
	LimitAmount string `json:"limit_amount"`
	SpentAmount string `json:"spent_amount"`
	Status      string `json:"status"`
}

type budgetRequestResponse struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id"`
	TenantName      string  `json:"tenant_name"`
	RequestedAmount string  `json:"requested_amount"`
	Reason          string  `json:"reason"`
	Status          string  `json:"status"`
	ReviewerID      *string `json:"reviewer_id"`
	ReviewedAt      *string `json:"reviewed_at"`
	CreatedAt       string  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Admin: platform-wide budget oversight and request approvals.
// ---------------------------------------------------------------------------

func HandleListBudgets(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT b.id, b.tenant_id, COALESCE(t.name, ''), b.period,
			        b.limit_amount::text, b.spent_amount::text, b.status
			 FROM budgets b LEFT JOIN tenants t ON t.id = b.tenant_id
			 ORDER BY b.created_at ASC`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list budgets"})
			return
		}
		defer rows.Close()
		out := make([]budgetResponse, 0)
		for rows.Next() {
			var b budgetResponse
			if err := rows.Scan(&b.ID, &b.TenantID, &b.TenantName, &b.Period,
				&b.LimitAmount, &b.SpentAmount, &b.Status); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read budget"})
				return
			}
			out = append(out, b)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

func HandleListBudgetRequests(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		requests, err := a.Budgets.ListRequests(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list budget requests"})
			return
		}
		out := make([]budgetRequestResponse, 0, len(requests))
		for _, req := range requests {
			name := tenantName(a, r, req.TenantID)
			out = append(out, budgetRequestResponseFromDomain(req, name))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

func HandleApproveBudgetRequest(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		requestID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request id"})
			return
		}
		reviewerID, _ := jwtutil.UserIDFromContext(r.Context())
		budget, err := a.Budgets.ApproveRequest(r.Context(), requestID, reviewerID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"budget_id":    budget.ID.String(),
			"limit_amount": budget.LimitAmount.String(),
		})
	}
}

func HandleRejectBudgetRequest(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		requestID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request id"})
			return
		}
		reviewerID, _ := jwtutil.UserIDFromContext(r.Context())
		if err := a.Budgets.RejectRequest(r.Context(), requestID, reviewerID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"rejected": true})
	}
}

// ---------------------------------------------------------------------------
// Enterprise: view budget and request increases.
// ---------------------------------------------------------------------------

func HandleGetTeamBudget(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}
		budgets, err := a.Budgets.ListByTenant(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list budgets"})
			return
		}
		requests, err := a.Budgets.ListRequestsByTenant(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list budget requests"})
			return
		}
		tenantName := tenantName(a, r, tenantID)
		outBudgets := make([]budgetResponse, 0, len(budgets))
		for _, b := range budgets {
			outBudgets = append(outBudgets, budgetResponse{
				ID: b.ID.String(), TenantID: b.TenantID.String(), TenantName: tenantName,
				Period: string(b.Period), LimitAmount: b.LimitAmount.String(),
				SpentAmount: b.SpentAmount.String(), Status: string(b.Status),
			})
		}
		outRequests := make([]budgetRequestResponse, 0, len(requests))
		for _, req := range requests {
			outRequests = append(outRequests, budgetRequestResponseFromDomain(req, tenantName))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"budgets": outBudgets, "requests": outRequests,
		})
	}
}

func HandleCreateBudgetRequest(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}
		var req struct {
			Amount string `json:"amount"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		amount, err := decimal.NewFromString(req.Amount)
		if err != nil || !amount.IsPositive() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amount must be a positive decimal"})
			return
		}
		request := &domain.BudgetRequest{
			TenantID: tenantID, RequestedAmount: amount, Reason: req.Reason,
			Status: domain.BudgetRequestPending,
		}
		if err := a.Budgets.CreateRequest(r.Context(), request); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create budget request"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": request.ID.String()})
	}
}

func budgetRequestResponseFromDomain(req domain.BudgetRequest, tenantName string) budgetRequestResponse {
	out := budgetRequestResponse{
		ID: req.ID.String(), TenantID: req.TenantID.String(), TenantName: tenantName,
		RequestedAmount: req.RequestedAmount.String(), Reason: req.Reason,
		Status: string(req.Status), CreatedAt: req.CreatedAt.Format(time.RFC3339),
	}
	if req.ReviewerID != nil {
		s := req.ReviewerID.String()
		out.ReviewerID = &s
	}
	if req.ReviewedAt != nil {
		s := req.ReviewedAt.Format(time.RFC3339)
		out.ReviewedAt = &s
	}
	return out
}

func tenantName(a *app.App, r *http.Request, tenantID uuid.UUID) string {
	var name string
	_ = a.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(name, '') FROM tenants WHERE id = $1`, tenantID).Scan(&name)
	return name
}
