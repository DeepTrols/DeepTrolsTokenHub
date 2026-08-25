package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func newVideoRequest(userID, apiKeyID uuid.UUID, body map[string]any) *http.Request {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-video-"+uuid.New().String())
	return setAuthContext(req, userID, apiKeyID)
}

func TestHandleVideoGenerations_SyncSettles(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK,
				Body: map[string]any{
					"data": []any{map[string]any{"url": "https://video.example.com/out.mp4"}},
				},
				Usage:       &usageparser.NormalizedUsage{},
				UsageSource: usageparser.SourceEstimated,
				DurationMs:  500,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newVideoRequest(userID, apiKeyID, map[string]any{
		"model":  "doubao-seedance",
		"prompt": "a cat jumping",
		"n":      float64(1),
	})
	w := httptest.NewRecorder()
	HandleVideoGenerations(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", resp.Status)
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "video" {
		t.Errorf("RequestType = %q, want video", log.RequestType)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "video" {
		t.Errorf("charge lines = %+v, want first dimension video", usageRepo.lastChargeLines)
	}
}

func TestHandleVideoGenerations_AsyncCreatesJob(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode:  http.StatusOK,
				Body:        map[string]any{"id": "upstream-task-42"},
				Usage:       &usageparser.NormalizedUsage{},
				UsageSource: usageparser.SourceEstimated,
				DurationMs:  120,
			}, nil
		},
	}
	application, walletRepo, _ := newEndpointEnv(executor)

	req := newVideoRequest(userID, apiKeyID, map[string]any{
		"model":  "doubao-seedance",
		"prompt": "ocean waves",
	})
	w := httptest.NewRecorder()
	HandleVideoGenerations(application).ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "processing" {
		t.Errorf("status = %q, want processing", resp.Status)
	}
	if resp.TaskID != "upstream-task-42" {
		t.Errorf("task_id = %q, want upstream-task-42", resp.TaskID)
	}
	if walletRepo.settleCalled != 0 || walletRepo.releaseCalled != 0 {
		t.Errorf("async job must keep hold: settle=%d release=%d", walletRepo.settleCalled, walletRepo.releaseCalled)
	}
}

func TestHandleVideoJobStatusAndCancel(t *testing.T) {
	pool := testutil.SetupPool(t)
	userID := uuid.New()
	apiKeyID := uuid.New()
	holdTxID := uuid.New()
	jobID := uuid.New()
	now := time.Now().UTC()

	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'x')`,
		userID, "video-user@test.local",
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		`INSERT INTO async_jobs (id, user_id, api_key_id, tenant_id, model, status, request_type, upstream_job_id, result_url, hold_tx_id, request_id, created_at, updated_at)
		 VALUES ($1,$2,$3,NULL,'doubao-seedance','processing','video','up-1','',$4,'req-video-1',$5,$5)`,
		jobID, userID, apiKeyID, holdTxID, now,
	)
	if err != nil {
		t.Fatalf("seed async job: %v", err)
	}

	walletRepo := &mockWalletRepo{
		releaseFn: func(ctx context.Context, txID uuid.UUID) error { return nil },
	}
	application := &app.App{
		Pool:    pool,
		Config:  &config.Config{},
		Charger: billing.NewCharger(walletRepo),
	}

	// GET status
	statusReq := httptest.NewRequest(http.MethodGet, "/v1/videos/generations/"+jobID.String(), nil)
	statusReq = setAuthContext(statusReq, userID, apiKeyID)
	statusW := httptest.NewRecorder()
	router := newVideoTestRouter(application)
	router.ServeHTTP(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body = %s", statusW.Code, statusW.Body.String())
	}
	var statusResp struct {
		Status string `json:"status"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(statusW.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if statusResp.Status != "processing" || statusResp.TaskID != "up-1" {
		t.Errorf("status resp = %+v", statusResp)
	}

	// DELETE cancel releases the hold and flips the job to cancelled.
	cancelReq := httptest.NewRequest(http.MethodDelete, "/v1/videos/generations/"+jobID.String(), nil)
	cancelReq = setAuthContext(cancelReq, userID, apiKeyID)
	cancelW := httptest.NewRecorder()
	router.ServeHTTP(cancelW, cancelReq)
	if cancelW.Code != http.StatusOK {
		t.Fatalf("cancel code = %d, want 200; body = %s", cancelW.Code, cancelW.Body.String())
	}
	if walletRepo.releaseCalled != 1 {
		t.Errorf("releaseCalled = %d, want 1", walletRepo.releaseCalled)
	}
	if walletRepo.lastReleaseTxID != holdTxID {
		t.Errorf("released tx = %s, want %s", walletRepo.lastReleaseTxID, holdTxID)
	}

	var storedStatus string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM async_jobs WHERE id = $1`, jobID,
	).Scan(&storedStatus); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if storedStatus != "cancelled" {
		t.Errorf("stored status = %q, want cancelled", storedStatus)
	}

	// Cancelling again conflicts.
	cancelW2 := httptest.NewRecorder()
	router.ServeHTTP(cancelW2, cancelReq)
	if cancelW2.Code != http.StatusConflict {
		t.Errorf("second cancel code = %d, want 409; body = %s", cancelW2.Code, cancelW2.Body.String())
	}
}

func TestHandleVideoGenerations_InvalidN(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{}
	application, _, _ := newEndpointEnv(executor)

	req := newVideoRequest(userID, apiKeyID, map[string]any{
		"model": "doubao-seedance",
		"n":     float64(0),
	})
	w := httptest.NewRecorder()
	HandleVideoGenerations(application).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

// newVideoTestRouter mounts the video job routes for handler tests.
func newVideoTestRouter(application *app.App) http.Handler {
	router := chi.NewRouter()
	router.Get("/v1/videos/generations/{id}", HandleVideoJobStatus(application))
	router.Delete("/v1/videos/generations/{id}", HandleVideoJobCancel(application))
	return router
}
