package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestHandleSetMarkup_IncrementsPriceVersion(t *testing.T) {
	a := appForModelsTest(t)
	ctx := context.Background()

	admin := seedUserForTenantsTest(t, a, "markup-admin@test.com", "pass", "Markup Admin")

	modelID := uuid.New()
	if _, err := a.Pool.Exec(ctx, `
		INSERT INTO models (id, code, provider, category, display_name, status, release_stage)
		VALUES ($1, 'gpt-4o', 'openai', 'chat', 'GPT-4o', 'active', 'GA')
	`, modelID); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	pricingID := uuid.New()
	if _, err := a.Pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, price_version)
		VALUES ($1, $2, 'chat', 'input', 'token', '2.50', 'CNY', '1.25', TRUE, 1)
	`, pricingID, modelID); err != nil {
		t.Fatalf("insert pricing: %v", err)
	}

	payload := `{"markup_rate": "2.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/pricing/markup", strings.NewReader(payload))
	req = setAdminCtx(req, admin.ID.String())
	w := httptest.NewRecorder()

	HandleSetMarkup(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		RowsUpdated int64 `json:"rows_updated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.RowsUpdated != 1 {
		t.Errorf("rows_updated = %d, want 1", resp.RowsUpdated)
	}

	var priceVersion int64
	var unitPrice string
	if err := a.Pool.QueryRow(ctx,
		`SELECT price_version, unit_price FROM model_pricing WHERE id = $1`, pricingID).Scan(&priceVersion, &unitPrice); err != nil {
		t.Fatalf("query pricing: %v", err)
	}
	if priceVersion != 2 {
		t.Errorf("price_version = %d, want 2 (incremented by markup)", priceVersion)
	}
	gotPrice, err := decimal.NewFromString(unitPrice)
	if err != nil {
		t.Fatalf("parse unit_price %q: %v", unitPrice, err)
	}
	wantPrice := decimal.NewFromFloat(2.5)
	if !gotPrice.Equal(wantPrice) {
		t.Errorf("unit_price = %q, want 2.5 (1.25 * 2.0)", unitPrice)
	}
}
