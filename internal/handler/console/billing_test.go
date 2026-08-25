package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/billing_sync"
	billingadapters "github.com/deeptrols/api/internal/billing_sync/adapters"
	billingpersistence "github.com/deeptrols/api/internal/billing_sync/persistence"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func appForBillingTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	cfg := &config.Config{
		Encryption: config.EncryptionConfig{Key: "0123456789abcdef0123456789abcdef"},
	}
	repo := billingpersistence.NewPostgresRepository(pool, []byte(cfg.Encryption.Key))
	return &app.App{
		Pool:            pool,
		Config:          cfg,
		BillingSyncRepo: repo,
		BillingSync: billingsync.NewService(repo,
			billingadapters.NewRegistry(&http.Client{Timeout: 5 * time.Second})),
		Healthy: true,
	}
}

func adminReq(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, uuid.New().String()))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "admin"))
	return req
}

func billingRouter(a *app.App) http.Handler {
	r := chi.NewRouter()
	r.Get("/billing/connectors", HandleListBillingConnectors(a))
	r.Post("/billing/connectors", HandleCreateBillingConnector(a))
	r.Get("/billing/connectors/{id}", HandleGetBillingConnector(a))
	r.Put("/billing/connectors/{id}", HandleUpdateBillingConnector(a))
	r.Delete("/billing/connectors/{id}", HandleDeleteBillingConnector(a))
	r.Post("/billing/connectors/{id}/test", HandleTestBillingConnector(a))
	r.Post("/billing/connectors/{id}/sync", HandleSyncBillingConnector(a))
	r.Get("/billing/connectors/{id}/records", HandleListBillingRecords(a))
	r.Get("/billing/connectors/{id}/runs", HandleListBillingSyncRuns(a))
	return r
}

func TestBillingConnectorCRUD(t *testing.T) {
	a := appForBillingTest(t)
	router := billingRouter(a)

	// Create.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodPost, "/billing/connectors", map[string]any{
		"name": "aliyun-prod", "type": "aliyun",
		"base_url": "https://billing.aliyuncs.com",
		"config":   map[string]string{"product_code": "dbaudit"},
		"credentials": map[string]string{
			"access_key_id": "ak", "access_key_secret": "sk",
		},
	}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID                    string   `json:"id"`
		CredentialsConfigured bool     `json:"credentials_configured"`
		CredentialFields      []string `json:"credential_fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" || !created.CredentialsConfigured {
		t.Fatalf("create response missing id/credentials: %s", w.Body.String())
	}

	// List.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodGet, "/billing/connectors", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}

	// Get must never leak credentials.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodGet, "/billing/connectors/"+created.ID, nil))
	if w.Code != http.StatusOK || bytes.Contains(w.Body.Bytes(), []byte("access_key_secret")) {
		t.Fatalf("get leaked credentials: %d %s", w.Code, w.Body.String())
	}

	// Update name.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodPut, "/billing/connectors/"+created.ID, map[string]any{
		"name": "aliyun-staging",
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("aliyun-staging")) {
		t.Errorf("update did not apply name: %s", w.Body.String())
	}

	// Invalid type rejected with 400.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodPost, "/billing/connectors", map[string]any{
		"name": "bad", "type": "openai", "base_url": "https://x.example",
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid type status = %d, want 400", w.Code)
	}

	// Sync on a connector with no reachable upstream records a failed run (or
	// errors) instead of panicking; either is acceptable, but it must not 500
	// with a server fault.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodPost, "/billing/connectors/"+created.ID+"/sync", nil))
	if w.Code == http.StatusInternalServerError {
		t.Fatalf("sync returned 500: %s", w.Body.String())
	}

	// Delete.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodDelete, "/billing/connectors/"+created.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodGet, "/billing/connectors/"+created.ID, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", w.Code)
	}
}

func TestBillingConnectorRequiresAdmin(t *testing.T) {
	a := appForBillingTest(t)
	router := billingRouter(a)
	req := httptest.NewRequest(http.MethodGet, "/billing/connectors", nil)
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, uuid.New().String()))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "user"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", w.Code)
	}
}
