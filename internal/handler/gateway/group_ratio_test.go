package gateway

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/handler/middleware"
	settingrepo "github.com/deeptrols/api/internal/repository/setting"
	"github.com/deeptrols/api/internal/repository/testutil"
	settingsvc "github.com/deeptrols/api/internal/service/setting"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeSettingRepoForGroup struct {
	entries []settingrepo.Entry
}

func (f *fakeSettingRepoForGroup) All(ctx context.Context) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeSettingRepoForGroup) Get(ctx context.Context, keys ...string) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeSettingRepoForGroup) Upsert(ctx context.Context, entries []settingrepo.Entry) error {
	f.entries = append(f.entries, entries...)
	return nil
}

func groupApp(groupsJSON string) *app.App {
	return &app.App{
		Settings: settingsvc.NewService(&fakeSettingRepoForGroup{
			entries: []settingrepo.Entry{
				{Key: settingsvc.KeyUserGroups, Value: json.RawMessage(groupsJSON)},
			},
		}),
	}
}

func TestGroupRatio_ResolvesByKeyGroup(t *testing.T) {
	a := groupApp(`[{"name":"vip","ratio":"0.8"},{"name":"enterprise","ratio":"0.6"}]`)
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxAPIKeyGroup, "vip"))
	if got := groupRatio(a, req); !got.Equal(decimal.NewFromFloat(0.8)) {
		t.Fatalf("ratio = %s, want 0.8", got)
	}
}

func TestGroupRatio_DefaultsToOne(t *testing.T) {
	a := groupApp(`[{"name":"vip","ratio":"0.8"}]`)

	// No group on the request.
	req := httptest.NewRequest("GET", "/", nil)
	if got := groupRatio(a, req); !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("no group ratio = %s, want 1", got)
	}
	// Unknown group.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.CtxAPIKeyGroup, "nope"))
	if got := groupRatio(a, req2); !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("unknown group ratio = %s, want 1", got)
	}
	// Malformed ratio falls back to 1 (never a zero/negative price).
	bad := groupApp(`[{"name":"vip","ratio":"abc"}]`)
	req3 := httptest.NewRequest("GET", "/", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), middleware.CtxAPIKeyGroup, "vip"))
	if got := groupRatio(bad, req3); !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("malformed ratio = %s, want 1", got)
	}
}

func TestVolumeRatio_MatchesMonthlyUsage(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	a := &app.App{
		Pool: pool,
		Settings: settingsvc.NewService(&fakeSettingRepoForGroup{
			entries: []settingrepo.Entry{
				{Key: settingsvc.KeyDiscountTiers, Value: json.RawMessage(
					`[{"min_tokens":1000000,"ratio":"0.95"},{"min_tokens":5000000,"ratio":"0.9"}]`)},
			},
		}),
	}

	userID := uuid.New()
	keyID := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, status, created_at, updated_at)
		 VALUES ($1, 'vol@example.com', 'x', 'active', NOW(), NOW())`, userID); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, created_at, updated_at)
		 VALUES ($1, $2, 'sk-', $3, 'sk-****vol', 'vol key', NOW(), NOW())`,
		keyID, userID, "hash-vol-"+uuid.New().String()[:8]); err != nil {
		t.Fatalf("api key: %v", err)
	}
	// 3M tokens this month → first tier (0.95) applies, not the 5M tier.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usage_logs (id, user_id, api_key_id, request_id, request_type,
		                         public_model_code, usage_source, usage_normalized, usage_raw,
		                         list_cost, final_cost, status, created_at)
		 VALUES ($1, $2, $3, $4, 'chat', 'deepseek-v4-flash', 'upstream',
		         '{"total_tokens": 3000000}'::jsonb, '{"total_tokens": 3000000}'::jsonb,
		         0, 1, 'completed', NOW())`,
		uuid.New(), userID, keyID, "req-vol-"+uuid.New().String()[:8]); err != nil {
		t.Fatalf("usage: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxUserID, userID.String()))
	if got := volumeRatio(a, req); !got.Equal(decimal.NewFromFloat(0.95)) {
		t.Fatalf("volume ratio = %s, want 0.95", got)
	}
}

func TestVolumeRatio_DefaultsToOne(t *testing.T) {
	a := groupApp(`[]`)
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxUserID, uuid.New().String()))
	if got := volumeRatio(a, req); !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("no tiers ratio = %s, want 1", got)
	}
}

func TestVolumeRatio_AppliesForNilUUIDAdmin(t *testing.T) {
	// The bootstrap admin owns the nil UUID; the volume discount must still
	// apply (identity presence is checked via context, not uuid.Nil).
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	a := &app.App{
		Pool: pool,
		Settings: settingsvc.NewService(&fakeSettingRepoForGroup{
			entries: []settingrepo.Entry{
				{Key: settingsvc.KeyDiscountTiers, Value: json.RawMessage(
					`[{"min_tokens":1000000,"ratio":"0.5"}]`)},
			},
		}),
	}
	adminID := uuid.Nil
	keyID := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, status, created_at, updated_at)
		 VALUES ($1, 'niladmin@example.com', 'x', 'active', NOW(), NOW())`, adminID); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, created_at, updated_at)
		 VALUES ($1, $2, 'sk-', $3, 'sk-****nil', 'nil key', NOW(), NOW())`,
		keyID, adminID, "hash-nil-"+uuid.New().String()[:8]); err != nil {
		t.Fatalf("api key: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usage_logs (id, user_id, api_key_id, request_id, request_type,
		                         public_model_code, usage_source, usage_normalized, usage_raw,
		                         list_cost, final_cost, status, created_at)
		 VALUES ($1, $2, $3, $4, 'chat', 'deepseek-v4-flash', 'upstream',
		         '{"total_tokens": 2000000}'::jsonb, '{"total_tokens": 2000000}'::jsonb,
		         0, 1, 'completed', NOW())`,
		uuid.New(), adminID, keyID, "req-nil-"+uuid.New().String()[:8]); err != nil {
		t.Fatalf("usage: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxUserID, adminID.String()))
	if got := volumeRatio(a, req); !got.Equal(decimal.NewFromFloat(0.5)) {
		t.Fatalf("nil-uuid admin ratio = %s, want 0.5", got)
	}
}
