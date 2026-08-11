package console

import (
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
)

// isPlatformTenant reports whether t is the auto-provisioned platform
// enterprise owned by the system administrator (see app.PlatformTenantCode).
func isPlatformTenant(t *domain.Tenant) bool {
	return t != nil && t.Code == app.PlatformTenantCode
}

// rejectPlatformTenantMutation writes a 403 when the target is the platform
// tenant and returns true so the caller can bail out. The platform tenant is
// bootstrap-owned and self-healing; destructive admin operations (delete,
// suspend/terminate, transfer ownership) must not be able to disable the
// system administrator's enterprise/team console features.
func rejectPlatformTenantMutation(w http.ResponseWriter, t *domain.Tenant) bool {
	if !isPlatformTenant(t) {
		return false
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "The platform tenant cannot be deleted or have its status or ownership changed",
	})
	return true
}
