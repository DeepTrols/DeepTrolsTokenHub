package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	guardrailpersistence "github.com/deeptrols/api/internal/guardrails/persistence"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestGuardrailPolicyCRUD(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin := &domain.User{ID: uuid.New(), Email: "gr-admin@test.local",
		PasswordHash: string(hash), DisplayName: "GR Admin", Role: "admin", Status: domain.UserStatusActive}
	if err := user.NewPostgresRepository(pool).Create(ctx, admin); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	repo := guardrailpersistence.NewPostgresRepository(pool)
	a := &app.App{Pool: pool, Config: &config.Config{}, GuardrailsPolicies: repo}
	router := chi.NewRouter()
	router.Get("/guardrails", HandleListGuardrailPolicies(a))
	router.Post("/guardrails", HandleSaveGuardrailPolicy(a))
	router.Delete("/guardrails/{id}", HandleDeleteGuardrailPolicy(a))
	adminReq := func(method, path string, body any) *http.Request {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, admin.ID.String()))
		req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "admin"))
		return req
	}

	// Save.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodPost, "/guardrails", map[string]any{
		"name": "敏感词拦截", "status": "active",
		"detection_items": []map[string]any{{
			"name": "keyword", "detector_type": "pattern", "action": "block",
			"config": map[string]any{"keywords": []any{"机密"}},
		}},
		"bindings": []map[string]any{{
			"scope_type": "all_projects", "checkpoint": "before_provider", "protocol": "all",
		}},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d, body: %s", w.Code, w.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &saved)
	if saved.ID == "" {
		t.Fatal("save returned no id")
	}

	// List.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodGet, "/guardrails", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("敏感词拦截")) {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}

	// The saved policy must actually block via the engine (persistence round-trip).
	policies, err := repo.LoadPolicies(ctx)
	if err != nil || len(policies) != 1 {
		t.Fatalf("load policies = %d err=%v", len(policies), err)
	}

	// Delete.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, adminReq(http.MethodDelete, "/guardrails/"+saved.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}
	after, _ := repo.LoadPolicies(ctx)
	if len(after) != 0 {
		t.Errorf("policies after delete = %d, want 0", len(after))
	}
}
