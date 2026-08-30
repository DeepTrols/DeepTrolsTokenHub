package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	settingrepo "github.com/deeptrols/api/internal/repository/setting"
	settingsvc "github.com/deeptrols/api/internal/service/setting"
	"github.com/google/uuid"
)

// TestHandlePublicPricing_IncludesTierConditions locks the public pricing
// contract: the unauthenticated /pricing payload must expose model_pricing
// conditions so the frontend can show tiered (min/max total tokens) prices,
// while never leaking cost rows or other sensitive fields.
func TestHandlePublicPricing_IncludesTierConditions(t *testing.T) {
	a := appForModelsTest(t)
	ctx := context.Background()

	modelID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'deepseek-tiers', 'deepseek', 'chat', 'DeepSeek Tiers', 128000, 'active', 'GA', NOW(), NOW())`,
		modelID); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	// Only channel-backed models are routable and appear in the public catalog.
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, 'tiers-channel', $2, 'shared', 'active', NOW(), NOW())`,
		uuid.New(), modelID); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	for _, row := range []struct {
		dim, price, cond string
	}{
		{"input", "3.00", `{"max_total_tokens":200000}`},
		{"input", "2.00", `{"min_total_tokens":200001}`},
		{"output", "8.00", ""},
	} {
		conds := "NULL"
		if row.cond != "" {
			conds = "'" + row.cond + "'::jsonb"
		}
		if _, err := a.Pool.Exec(ctx,
			`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active, period, price_type, conditions, created_at, updated_at)
			 VALUES ($1, $2, 'chat', $3, '1M tokens', $4, 'CNY', TRUE, 'off_peak', 'sell', `+conds+`, NOW(), NOW())`,
			uuid.New(), modelID, row.dim, row.price); err != nil {
			t.Fatalf("insert pricing %s: %v", row.dim, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/pricing", nil)
	w := httptest.NewRecorder()
	HandlePublicPricing(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Code     string `json:"code"`
			Pricings []struct {
				Dimension  string         `json:"dimension"`
				UnitPrice  string         `json:"unit_price"`
				PriceType  string         `json:"price_type"`
				Conditions map[string]any `json:"conditions"`
			} `json:"pricings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.Data))
	}
	m := resp.Data[0]
	if m.Code != "deepseek-tiers" {
		t.Fatalf("code = %s", m.Code)
	}
	inputTiers := 0
	for _, p := range m.Pricings {
		if p.PriceType != "sell" {
			t.Errorf("unexpected price_type %q leaked", p.PriceType)
		}
		if p.Dimension != "input" {
			continue
		}
		inputTiers++
		if trimDecimalPrice(p.UnitPrice) == "2.00" {
			if p.Conditions["min_total_tokens"] != float64(200001) {
				t.Errorf("input 2.00 conditions = %v, want min_total_tokens 200001", p.Conditions)
			}
		}
		if trimDecimalPrice(p.UnitPrice) == "3.00" {
			if p.Conditions["max_total_tokens"] != float64(200000) {
				t.Errorf("input 3.00 conditions = %v, want max_total_tokens 200000", p.Conditions)
			}
		}
	}
	if inputTiers != 2 {
		t.Errorf("exposed %d input tiers, want 2", inputTiers)
	}
}

// TestHandlePublicPricing_HiddenWhenDisabled verifies the public pricing gate:
// models_public_visible=false returns an empty catalog, never 500s.
func TestHandlePublicPricing_HiddenWhenDisabled(t *testing.T) {
	a := appForModelsTest(t)
	a.Settings = settingsvc.NewService(&fakeOAuthSettingRepo{entries: []settingrepo.Entry{
		{Key: "models_public_visible", Value: json.RawMessage(`false`)},
	}})

	req := httptest.NewRequest(http.MethodGet, "/api/public/pricing", nil)
	w := httptest.NewRecorder()
	HandlePublicPricing(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data  []any `json:"data"`
		Total int   `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("expected empty catalog, got total=%d data=%d", resp.Total, len(resp.Data))
	}
}
