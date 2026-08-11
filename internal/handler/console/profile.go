package console

import (
	"errors"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/user"
)

type profileResponse struct {
	User       meResponse      `json:"user"`
	Enterprise *enterpriseInfo `json:"enterprise,omitempty"`
}

type enterpriseInfo struct {
	TenantID   string       `json:"tenant_id"`
	TenantName string       `json:"tenant_name"`
	CreditCode string       `json:"credit_code"`
	Members    []memberItem `json:"members"`
}

type memberItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// HandleGetProfile returns the full user profile including enterprise info.
func HandleGetProfile(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		dbUser, err := a.Users.FindByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "User not found"})
				return
			}
			log.Printf("HandleGetProfile: FindByID error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}

		resp := profileResponse{
			User: meResponse{
				ID:        dbUser.ID.String(),
				Email:     dbUser.Email,
				Name:      dbUser.DisplayName,
				Role:      dbUser.Role,
				Status:    string(dbUser.Status),
				UserType:  string(dbUser.UserType),
				Phone:     dbUser.Phone,
				AvatarURL: dbUser.AvatarURL,
			},
		}

		// Look up enterprise membership.
		m, err := a.Memberships.FindByUserID(r.Context(), dbUser.ID)
		if err == nil && m != nil && m.Status == domain.MembershipStatusActive {
			resp.User.TenantID = m.TenantID.String()
			resp.User.TenantRole = string(m.Role)

			tenant, err := a.Tenants.FindByID(r.Context(), m.TenantID)
			if err == nil && tenant != nil {
				resp.User.TenantName = tenant.Name
				resp.Enterprise = &enterpriseInfo{
					TenantID:   tenant.ID.String(),
					TenantName: tenant.Name,
					CreditCode: tenant.CreditCode,
				}

				// List team members with user details.
				members, err := a.Memberships.FindByTenantID(r.Context(), m.TenantID)
				if err == nil {
					for _, member := range members {
						if member.Status != domain.MembershipStatusActive {
							continue
						}
						item := memberItem{
							ID:   member.UserID.String(),
							Role: string(member.Role),
						}
						if u, err := a.Users.FindByID(r.Context(), member.UserID); err == nil && u != nil {
							item.Name = u.DisplayName
							item.Email = u.Email
						}
						resp.Enterprise.Members = append(resp.Enterprise.Members, item)
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
