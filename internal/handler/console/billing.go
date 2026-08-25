package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/billing_sync"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Billing connector management (admin only). Backs the Step 1a billing sync
// UI: configure OneAPI / NewAPI / Aliyun connectors, test, trigger sync, and
// browse the resulting records and sync runs.
// ---------------------------------------------------------------------------

type billingConnectorRequest struct {
	Name                    string            `json:"name"`
	Type                    string            `json:"type"`
	BaseURL                 string            `json:"base_url"`
	Status                  string            `json:"status"`
	ScheduleIntervalMinutes int               `json:"schedule_interval_minutes"`
	Config                  map[string]string `json:"config"`
	Credentials             map[string]string `json:"credentials"`
}

type billingConnectorResponse struct {
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	Type                    string            `json:"type"`
	BaseURL                 string            `json:"base_url"`
	Status                  string            `json:"status"`
	ScheduleIntervalMinutes int               `json:"schedule_interval_minutes"`
	Config                  map[string]string `json:"config"`
	CredentialsConfigured   bool              `json:"credentials_configured"`
	CredentialFields        []string          `json:"credential_fields"`
	Checkpoint              string            `json:"checkpoint"`
	LastSyncedThrough       *string           `json:"last_synced_through"`
	LastSyncStatus          string            `json:"last_sync_status"`
	LastSyncMessage         string            `json:"last_sync_message"`
	LastSyncAt              *string           `json:"last_sync_at"`
	NextSyncAt              *string           `json:"next_sync_at"`
	CreatedAt               string            `json:"created_at"`
	UpdatedAt               string            `json:"updated_at"`
}

type billingRecordResponse struct {
	ID                string            `json:"id"`
	ExternalID        string            `json:"external_id"`
	SourceType        string            `json:"source_type"`
	AccountID         string            `json:"account_id"`
	Service           string            `json:"service"`
	Product           string            `json:"product"`
	Model             string            `json:"model"`
	Currency          string            `json:"currency"`
	GrossAmount       string            `json:"gross_amount"`
	DiscountAmount    string            `json:"discount_amount"`
	TaxAmount         string            `json:"tax_amount"`
	RefundAmount      string            `json:"refund_amount"`
	NetAmount         string            `json:"net_amount"`
	UsageQuantity     int64             `json:"usage_quantity"`
	UsageUnit         string            `json:"usage_unit"`
	UsageStartAt      string            `json:"usage_start_at"`
	UsageEndAt        string            `json:"usage_end_at"`
	BillingPeriod     string            `json:"billing_period"`
	ExternalRequestID string            `json:"external_request_id"`
	Metadata          map[string]string `json:"metadata"`
	CreatedAt         string            `json:"created_at"`
}

type billingSyncRunResponse struct {
	ID              string  `json:"id"`
	Trigger         string  `json:"trigger"`
	Status          string  `json:"status"`
	RangeStart      string  `json:"range_start"`
	RangeEnd        string  `json:"range_end"`
	PagesFetched    int     `json:"pages_fetched"`
	RecordsSeen     int     `json:"records_seen"`
	RecordsInserted int     `json:"records_inserted"`
	RecordsUpdated  int     `json:"records_updated"`
	ErrorCode       string  `json:"error_code"`
	ErrorMessage    string  `json:"error_message"`
	StartedAt       string  `json:"started_at"`
	FinishedAt      *string `json:"finished_at"`
}

func HandleListBillingConnectors(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		connectors := a.BillingSyncRepo.ListBillingConnectors()
		out := make([]billingConnectorResponse, 0, len(connectors))
		for _, c := range connectors {
			out = append(out, billingConnectorResponseFromDomain(c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

func HandleCreateBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var req billingConnectorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		connector, err := billingsync.NormalizeConnector(billingsync.ConnectorInput{
			Name:                    req.Name,
			Type:                    req.Type,
			BaseURL:                 req.BaseURL,
			Status:                  req.Status,
			ScheduleIntervalMinutes: req.ScheduleIntervalMinutes,
			Config:                  req.Config,
			Credentials:             req.Credentials,
		})
		if err != nil {
			writeBillingError(w, err)
			return
		}
		created, err := a.BillingSyncRepo.CreateBillingConnector(connector)
		if err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, billingConnectorResponseFromDomain(created))
	}
}

func HandleGetBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		id := chi.URLParam(r, "id")
		connector, err := a.BillingSyncRepo.GetBillingConnector(id, false)
		if err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, billingConnectorResponseFromDomain(connector))
	}
}

func HandleUpdateBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		id := chi.URLParam(r, "id")
		var req billingConnectorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		current, err := a.BillingSyncRepo.GetBillingConnector(id, false)
		if err != nil {
			writeBillingError(w, err)
			return
		}
		patch, err := billingsync.NormalizeConnectorPatch(billingsync.ConnectorPatchInput{
			Name:                    stringPtr(req.Name),
			BaseURL:                 stringPtr(req.BaseURL),
			Status:                  stringPtr(req.Status),
			ScheduleIntervalMinutes: intPtr(req.ScheduleIntervalMinutes),
			Config:                  req.Config,
			Credentials:             req.Credentials,
		}, current)
		if err != nil {
			writeBillingError(w, err)
			return
		}
		updated, err := a.BillingSyncRepo.UpdateBillingConnector(id, patch)
		if err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, billingConnectorResponseFromDomain(updated))
	}
}

func HandleDeleteBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		if err := a.BillingSyncRepo.DeleteBillingConnector(chi.URLParam(r, "id")); err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func HandleTestBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		result, err := a.BillingSync.Test(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func HandleSyncBillingConnector(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		run, err := a.BillingSync.Sync(r.Context(), chi.URLParam(r, "id"), billingsync.SyncRequest{}, "manual")
		if err != nil {
			writeBillingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, billingSyncRunResponseFromDomain(run))
	}
}

func HandleListBillingRecords(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		records := a.BillingSyncRepo.ListBillingRecords(chi.URLParam(r, "id"), 200)
		out := make([]billingRecordResponse, 0, len(records))
		for _, rec := range records {
			out = append(out, billingRecordResponseFromDomain(rec))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

func HandleListBillingSyncRuns(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		runs := a.BillingSyncRepo.ListBillingSyncRuns(chi.URLParam(r, "id"), 50)
		out := make([]billingSyncRunResponse, 0, len(runs))
		for _, run := range runs {
			out = append(out, billingSyncRunResponseFromDomain(run))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func billingConnectorResponseFromDomain(c billingsync.Connector) billingConnectorResponse {
	return billingConnectorResponse{
		ID:                      c.ID,
		Name:                    c.Name,
		Type:                    c.Type,
		BaseURL:                 c.BaseURL,
		Status:                  c.Status,
		ScheduleIntervalMinutes: c.ScheduleIntervalMinutes,
		Config:                  c.Config,
		CredentialsConfigured:   c.CredentialsConfigured,
		CredentialFields:        c.CredentialFields,
		Checkpoint:              c.Checkpoint,
		LastSyncedThrough:       timePtrString(c.LastSyncedThrough),
		LastSyncStatus:          c.LastSyncStatus,
		LastSyncMessage:         c.LastSyncMessage,
		LastSyncAt:              timePtrString(c.LastSyncAt),
		NextSyncAt:              timePtrString(c.NextSyncAt),
		CreatedAt:               c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:               c.UpdatedAt.Format(time.RFC3339),
	}
}

func billingRecordResponseFromDomain(rec billingsync.Record) billingRecordResponse {
	return billingRecordResponse{
		ID:                rec.ID,
		ExternalID:        rec.ExternalID,
		SourceType:        rec.SourceType,
		AccountID:         rec.AccountID,
		Service:           rec.Service,
		Product:           rec.Product,
		Model:             rec.Model,
		Currency:          rec.Currency,
		GrossAmount:       rec.GrossAmount,
		DiscountAmount:    rec.DiscountAmount,
		TaxAmount:         rec.TaxAmount,
		RefundAmount:      rec.RefundAmount,
		NetAmount:         rec.NetAmount,
		UsageQuantity:     rec.UsageQuantity,
		UsageUnit:         rec.UsageUnit,
		UsageStartAt:      rec.UsageStartAt.Format(time.RFC3339),
		UsageEndAt:        rec.UsageEndAt.Format(time.RFC3339),
		BillingPeriod:     rec.BillingPeriod,
		ExternalRequestID: rec.ExternalRequestID,
		Metadata:          rec.Metadata,
		CreatedAt:         rec.CreatedAt.Format(time.RFC3339),
	}
}

func billingSyncRunResponseFromDomain(run billingsync.SyncRun) billingSyncRunResponse {
	out := billingSyncRunResponse{
		ID:              run.ID,
		Trigger:         run.Trigger,
		Status:          run.Status,
		RangeStart:      run.RangeStart.Format(time.RFC3339),
		RangeEnd:        run.RangeEnd.Format(time.RFC3339),
		PagesFetched:    run.PagesFetched,
		RecordsSeen:     run.RecordsSeen,
		RecordsInserted: run.RecordsInserted,
		RecordsUpdated:  run.RecordsUpdated,
		ErrorCode:       run.ErrorCode,
		ErrorMessage:    run.ErrorMessage,
		StartedAt:       run.StartedAt.Format(time.RFC3339),
		FinishedAt:      timePtrString(run.FinishedAt),
	}
	return out
}

func timePtrString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(v int) *int {
	return &v
}

func writeBillingError(w http.ResponseWriter, err error) {
	kind, code, message, ok := billingsync.ErrorInfo(err)
	if !ok {
		if errors.Is(err, context.DeadlineExceeded) {
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "billing operation timed out"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusInternalServerError
	switch kind {
	case billingsync.ErrorInvalidInput:
		status = http.StatusBadRequest
	case billingsync.ErrorNotFound:
		status = http.StatusNotFound
	case billingsync.ErrorConflict:
		status = http.StatusConflict
	case billingsync.ErrorRateLimited:
		status = http.StatusTooManyRequests
	case billingsync.ErrorUpstream, billingsync.ErrorTimeout:
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
