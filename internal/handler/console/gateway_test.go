package console

import (
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
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestHandleGatewayHealth(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin := &domain.User{ID: uuid.New(), Email: "gh-admin@test.local",
		PasswordHash: string(hash), DisplayName: "GH Admin", Role: "admin", Status: domain.UserStatusActive}
	if err := user.NewPostgresRepository(pool).Create(ctx, admin); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	modelID := uuid.New()
	channelID := uuid.New()
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, status) VALUES ($1, 'deepseek-chat', 'deepseek', 'chat', 'active')`,
		modelID); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, strategy)
		 VALUES ($1, '主渠道', $2, 'shared', 100, 'healthy', 'active', 100, 10, 'priority_only')`,
		channelID, modelID); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, current_load, concurrency_limit, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'serverless', 'https://api.deepseek.com', 'deepseek-chat', 3, 10, '{}', 'active', $3, $3)`,
		uuid.New(), channelID, now); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	a := &app.App{Pool: pool, Config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/gateway/health", nil)
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, admin.ID.String()))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "admin"))
	w := httptest.NewRecorder()

	HandleGatewayHealth(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []gatewayHealthRow `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ChannelName != "主渠道" {
		t.Fatalf("data = %+v", resp.Data)
	}
	if resp.Data[0].CurrentLoad == nil || *resp.Data[0].CurrentLoad != 3 {
		t.Errorf("current_load = %v, want 3", resp.Data[0].CurrentLoad)
	}
}
