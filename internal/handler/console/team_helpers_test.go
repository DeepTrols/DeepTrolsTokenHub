package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/quota"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
)

// appForTeamTest wires the repos needed by the team handlers.
func appForTeamTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	return &app.App{
		Pool: pool,
		Config: &config.Config{
			JWT: config.JWTConfig{
				Secret:      "test-jwt-secret-enterprise-32-byte",
				ExpiryHours: 24,
			},
		},
		Users:       user.NewPostgresRepository(pool),
		Tenants:     tenant.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Quotas:      quota.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// setTenantCtx adds the user and tenant identity to the request context,
// mimicking what ConsoleAuth injects. An empty tenantID represents a personal
// user without a tenant.
func setTenantCtx(r *http.Request, userID, tenantID, tenantRole string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxTenantIDKey, tenantID)
	ctx = context.WithValue(ctx, jwtutil.CtxTenantRoleKey, tenantRole)
	return r.WithContext(ctx)
}

// seedTeamUser creates a user of type enterprise.
func seedTeamUser(t *testing.T, a *app.App, email string) *domain.User {
	t.Helper()
	return seedUserForLedgerTest(t, a, email, domain.UserTypeEnterprise)
}
