package console

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// tenantListResponse is the JSON shape for a tenant in list responses.
type tenantListResponse struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	OwnerID      *string `json:"owner_id,omitempty"`
	StatusReason string  `json:"status_reason,omitempty"`
	MemberCount  int     `json:"member_count"`
	CreatedAt    string  `json:"created_at"`
}

// tenantDetailResponse is the JSON shape for a single tenant with domains.
type tenantDetailResponse struct {
	ID               string           `json:"id"`
	Code             string           `json:"code"`
	Name             string           `json:"name"`
	Status           string           `json:"status"`
	OwnerID          *string          `json:"owner_id,omitempty"`
	BrandConfig      map[string]any   `json:"brand_config,omitempty"`
	RuntimeConfig    map[string]any   `json:"runtime_config,omitempty"`
	SettlementConfig map[string]any   `json:"settlement_config,omitempty"`
	StatusReason     string           `json:"status_reason,omitempty"`
	CreatedAt        string           `json:"created_at"`
	UpdatedAt        string           `json:"updated_at,omitempty"`
	Domains          []domainResponse `json:"domains,omitempty"`
}

// domainResponse is the JSON shape for a tenant domain.
type domainResponse struct {
	ID        uuid.UUID `json:"id"`
	Domain    string    `json:"domain"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt string    `json:"created_at,omitempty"`
}

// createTenantRequest is the request body for HandleCreateTenant.
type createTenantRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	OwnerID string `json:"owner_id,omitempty"`
}

// updateTenantRequest is the request body for HandleUpdateTenant.
type updateTenantRequest struct {
	Name         *string        `json:"name,omitempty"`
	Status       *string        `json:"status,omitempty"`
	StatusReason *string        `json:"status_reason,omitempty"`
	BrandConfig  map[string]any `json:"brand_config,omitempty"`
}

// addDomainRequest is the request body for HandleAddTenantDomain.
type addDomainRequest struct {
	Domain    string `json:"domain"`
	IsPrimary *bool  `json:"is_primary,omitempty"`
}

// HandleListTenants returns all tenants ordered by creation date descending.
func HandleListTenants(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenants, err := a.Tenants.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list tenants"})
			return
		}

		// Count active members per tenant with a single aggregate query so the
		// list can show member counts without N+1 lookups. Only active members
		// are counted, matching the member_count returned by GET /enterprise.
		memberCounts := make(map[uuid.UUID]int, len(tenants))
		rows, err := a.Pool.Query(r.Context(),
			`SELECT tenant_id, COUNT(*) FROM tenant_memberships WHERE status = 'active' GROUP BY tenant_id`)
		if err != nil {
			log.Printf("HandleListTenants: member count query: %v", err)
		} else {
			for rows.Next() {
				var tenantID uuid.UUID
				var n int
				if err := rows.Scan(&tenantID, &n); err != nil {
					log.Printf("HandleListTenants: member count scan: %v", err)
					continue
				}
				memberCounts[tenantID] = n
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				log.Printf("HandleListTenants: member count rows: %v", err)
			}
		}

		response := make([]tenantListResponse, 0, len(tenants))
		for _, t := range tenants {
			item := tenantListResponse{
				ID:           t.ID.String(),
				Code:         t.Code,
				Name:         t.Name,
				Status:       string(t.Status),
				StatusReason: t.StatusReason,
				MemberCount:  memberCounts[t.ID],
				CreatedAt:    t.CreatedAt.Format(time.RFC3339),
			}
			if t.OwnerID != nil {
				ownerStr := t.OwnerID.String()
				item.OwnerID = &ownerStr
			}
			response = append(response, item)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleGetTenant returns a single tenant with all its domains.
func HandleGetTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		domains := queryTenantDomains(r.Context(), a, tenantID)

		detail := tenantDetailResponse{
			ID:               tn.ID.String(),
			Code:             tn.Code,
			Name:             tn.Name,
			Status:           string(tn.Status),
			BrandConfig:      tn.BrandConfig,
			RuntimeConfig:    tn.RuntimeConfig,
			SettlementConfig: tn.SettlementConfig,
			StatusReason:     tn.StatusReason,
			CreatedAt:        tn.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        tn.UpdatedAt.Format(time.RFC3339),
			Domains:          domains,
		}
		if tn.OwnerID != nil {
			ownerStr := tn.OwnerID.String()
			detail.OwnerID = &ownerStr
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": detail,
		})
	}
}

// HandleCreateTenant creates a new tenant with status "pending_review".
func HandleCreateTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req createTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}

		// Check code uniqueness
		existing, err := a.Tenants.FindByCode(r.Context(), req.Code)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("HandleCreateTenant: FindByCode error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}
		if existing != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Tenant code already exists"})
			return
		}

		now := time.Now().UTC()
		tn := &domain.Tenant{
			ID:        uuid.New(),
			Code:      req.Code,
			Name:      req.Name,
			Status:    domain.TenantStatusPendingReview,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if req.OwnerID != "" {
			ownerID, err := uuid.Parse(req.OwnerID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid owner_id"})
				return
			}
			tn.OwnerID = &ownerID
		}

		if err := a.Tenants.Create(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": tenantListResponse{
				ID:        tn.ID.String(),
				Code:      tn.Code,
				Name:      tn.Name,
				Status:    string(tn.Status),
				CreatedAt: tn.CreatedAt.Format(time.RFC3339),
			},
		})
	}
}

// HandleUpdateTenant updates a tenant's fields including status with transition validation.
func HandleUpdateTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		var req updateTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		// Apply field updates
		if req.Name != nil {
			tn.Name = *req.Name
		}
		if req.StatusReason != nil {
			tn.StatusReason = *req.StatusReason
		}
		if req.BrandConfig != nil {
			tn.BrandConfig = req.BrandConfig
		}

		// Status change with transition validation
		if req.Status != nil {
			newStatus := domain.TenantStatus(*req.Status)

			// Validate it's a known status
			if !isValidTenantStatus(newStatus) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status value"})
				return
			}

			// If status actually changes, validate the transition
			if newStatus != tn.Status {
				if !isValidTenantTransition(tn, newStatus) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status transition"})
					return
				}
				tn.Status = newStatus
			}
		}

		tn.UpdatedAt = time.Now().UTC()

		if err := a.Tenants.Update(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update tenant"})
			return
		}

		resp := tenantDetailResponse{
			ID:           tn.ID.String(),
			Code:         tn.Code,
			Name:         tn.Name,
			Status:       string(tn.Status),
			StatusReason: tn.StatusReason,
			UpdatedAt:    tn.UpdatedAt.Format(time.RFC3339),
		}
		if tn.OwnerID != nil {
			ownerStr := tn.OwnerID.String()
			resp.OwnerID = &ownerStr
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": resp,
		})
	}
}

// HandleDeleteTenant terminates a tenant by setting status to "terminated".
func HandleDeleteTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		tn.Status = domain.TenantStatusTerminated
		tn.StatusReason = "admin action"
		tn.UpdatedAt = time.Now().UTC()

		if err := a.Tenants.Update(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to terminate tenant"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "terminated",
			"id":     tn.ID.String(),
		})
	}
}

// HandleAddTenantDomain adds a domain to a tenant.
func HandleAddTenantDomain(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		var req addDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
			return
		}

		// Verify tenant exists
		_, err = a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		isPrimary := false
		if req.IsPrimary != nil {
			isPrimary = *req.IsPrimary
		}

		domainID := uuid.New()
		now := time.Now().UTC()

		_, err = a.Pool.Exec(r.Context(),
			`INSERT INTO tenant_domains (id, tenant_id, domain, is_primary, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			domainID, tenantID, req.Domain, isPrimary, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to add domain"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": domainResponse{
				ID:        domainID,
				Domain:    req.Domain,
				IsPrimary: isPrimary,
				CreatedAt: now.Format(time.RFC3339),
			},
		})
	}
}

// HandleRemoveTenantDomain removes a domain from a tenant.
func HandleRemoveTenantDomain(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		domainID, err := uuid.Parse(chi.URLParam(r, "domainId"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid domain ID"})
			return
		}

		// Verify tenant exists
		_, err = a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		// Verify domain exists and belongs to tenant
		var count int
		err = a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM tenant_domains WHERE id = $1 AND tenant_id = $2`,
			domainID, tenantID,
		).Scan(&count)
		if err != nil || count == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Domain not found"})
			return
		}

		_, err = a.Pool.Exec(r.Context(),
			`DELETE FROM tenant_domains WHERE id = $1 AND tenant_id = $2`,
			domainID, tenantID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove domain"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "removed",
			"id":     domainID.String(),
		})
	}
}

// queryTenantDomains fetches all domains for a tenant.
func queryTenantDomains(ctx context.Context, a *app.App, tenantID uuid.UUID) []domainResponse {
	rows, err := a.Pool.Query(ctx,
		`SELECT id, domain, is_primary, created_at FROM tenant_domains WHERE tenant_id = $1 ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		log.Printf("queryTenantDomains: query error: %v", err)
		return nil
	}
	defer rows.Close()

	var domains []domainResponse
	for rows.Next() {
		var d domainResponse
		var createdAt time.Time
		if err := rows.Scan(&d.ID, &d.Domain, &d.IsPrimary, &createdAt); err != nil {
			log.Printf("queryTenantDomains: scan error: %v", err)
			return nil
		}
		d.CreatedAt = createdAt.Format(time.RFC3339)
		domains = append(domains, d)
	}
	return domains
}

// isValidTenantStatus returns true if the status is one of the defined TenantStatus values.
func isValidTenantStatus(s domain.TenantStatus) bool {
	switch s {
	case domain.TenantStatusPendingReview,
		domain.TenantStatusActive,
		domain.TenantStatusSuspended,
		domain.TenantStatusTerminated,
		domain.TenantStatusRejected:
		return true
	default:
		return false
	}
}

// isValidTenantTransition checks if the transition from the tenant's current status to the new status is allowed.
func isValidTenantTransition(tn *domain.Tenant, newStatus domain.TenantStatus) bool {
	for _, allowed := range tn.ValidTransitions() {
		if allowed == newStatus {
			return true
		}
	}
	return false
}
