package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/config"
)

// TestPublicStatsHandler_ReturnsModelCount verifies the public stats endpoint
// answers 200 with a non-negative model count (integration, test DB).
func TestPublicStatsHandler_ReturnsModelCount(t *testing.T) {
	cfg := &config.Config{DB: config.DBConfig{URL: testDBURL(t)}}
	application, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer application.Shutdown()

	req := httptest.NewRequest(http.MethodGet, "/api/public/stats", nil)
	rec := httptest.NewRecorder()
	PublicStatsHandler(application).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Models int64 `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Models < 0 {
		t.Errorf("models = %d, want >= 0", body.Models)
	}
}
