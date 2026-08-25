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
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/channel"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestHandleSimulateRouting(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin := &domain.User{ID: uuid.New(), Email: "sim-admin@test.local",
		PasswordHash: string(hash), DisplayName: "Sim Admin", Role: "admin", Status: domain.UserStatusActive}
	if err := user.NewPostgresRepository(pool).Create(ctx, admin); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	modelID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, status) VALUES ($1, 'deepseek-chat', 'deepseek', 'chat', 'active')`,
		modelID); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, strategy)
		 VALUES ($1, 'DeepSeek 主渠道', $2, 'shared', 100, 'healthy', 'active', 100, 10, 'priority_only')`,
		channelID, modelID); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'serverless', 'https://api.deepseek.com', 'deepseek-chat', '{}', 'active', $3, $3)`,
		instanceID, channelID, now); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	a := &app.App{
		Pool:   pool,
		Config: &config.Config{},
		Router: gw.NewRouter(model.NewPostgresRepository(pool), channel.NewPostgresRepository(pool)),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routing/simulate",
		bytes.NewBufferString(`{"model":"deepseek-chat"}`))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, admin.ID.String()))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "admin"))
	w := httptest.NewRecorder()

	HandleSimulateRouting(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data  []simulatedRoute `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("total = %d, want 1", resp.Total)
	}
	if resp.Data[0].ChannelID != channelID.String() || resp.Data[0].UpstreamModel != "deepseek-chat" {
		t.Errorf("route = %+v", resp.Data[0])
	}

	// Unknown model → 404.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/admin/routing/simulate",
		bytes.NewBufferString(`{"model":"nope-model"}`))
	req2 = req2.WithContext(req.Context())
	HandleSimulateRouting(a).ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unknown model status = %d, want 404", w2.Code)
	}
}
