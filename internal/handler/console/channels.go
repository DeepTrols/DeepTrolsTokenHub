package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// channelResponse is the JSON shape returned for a single channel in list responses.
type channelResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ModelID        string `json:"model_id"`
	ModelCode      string `json:"model_code"`
	Provider       string `json:"provider"`
	PoolType       string `json:"pool_type"`
	HealthScore    int    `json:"health_score"`
	HealthStatus   string `json:"health_status"`
	Status         string `json:"status"`
	Weight         int    `json:"weight"`
	MaxConcurrency int    `json:"max_concurrency"`
	InstanceCount  int    `json:"instance_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// channelDetailResponse extends channelResponse and includes instance details.
type channelDetailResponse struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	ModelID        string             `json:"model_id"`
	PoolType       string             `json:"pool_type"`
	HealthScore    int                `json:"health_score"`
	HealthStatus   string             `json:"health_status"`
	Status         string             `json:"status"`
	Weight         int                `json:"weight"`
	MaxConcurrency int                `json:"max_concurrency"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
	Instances      []instanceResponse `json:"instances"`
}

// instanceResponse is the JSON shape for a channel_instance.
type instanceResponse struct {
	ID           string `json:"id"`
	ChannelID    string `json:"channel_id"`
	InstanceType string `json:"instance_type"`
	BaseURL      string `json:"base_url"`
	CurrentLoad  int    `json:"current_load"`
	MaxLoad      int    `json:"max_load"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// createChannelRequest is the request body for HandleCreateChannel.
type createChannelRequest struct {
	Name           string `json:"name"`
	ModelID        string `json:"model_id"`
	PoolType       string `json:"pool_type"`
	Weight         *int   `json:"weight"`
	MaxConcurrency *int   `json:"max_concurrency"`
}

// updateChannelRequest is the request body for HandleUpdateChannel.
type updateChannelRequest struct {
	Name           *string `json:"name"`
	Weight         *int    `json:"weight"`
	MaxConcurrency *int    `json:"max_concurrency"`
	PoolType       *string `json:"pool_type"`
}

// addInstanceRequest is the request body for HandleAddInstance.
type addInstanceRequest struct {
	InstanceType string          `json:"instance_type"`
	BaseURL      string          `json:"base_url"`
	MaxLoad      *int            `json:"max_load"`
	Config       json.RawMessage `json:"config"`
}

// HandleListChannels returns all channels with instance counts.
func HandleListChannels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT c.id, c.name, c.model_id, m.code, m.provider,
			        c.pool_type, c.health_score, c.health_status,
			        c.status, c.weight, c.max_concurrency, c.created_at, c.updated_at,
			        COUNT(ci.id) AS instance_count
			 FROM channels c
			 JOIN models m ON m.id = c.model_id
			 LEFT JOIN channel_instances ci ON ci.channel_id = c.id
			 GROUP BY c.id, m.code, m.provider
			 ORDER BY m.provider, c.name`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query channels"})
			return
		}
		defer rows.Close()

		response := make([]channelResponse, 0)
		for rows.Next() {
			var (
				id                                                 uuid.UUID
				name, modelCode, provider, poolType                string
				healthStatus, status                               string
				modelID                                            uuid.UUID
				healthScore, weight, maxConcurrency, instanceCount int
				createdAt, updatedAt                               time.Time
			)
			if err := rows.Scan(&id, &name, &modelID, &modelCode, &provider,
				&poolType, &healthScore, &healthStatus,
				&status, &weight, &maxConcurrency, &createdAt, &updatedAt, &instanceCount); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read channel"})
				return
			}

			response = append(response, channelResponse{
				ID:             id.String(),
				Name:           name,
				ModelID:        modelID.String(),
				ModelCode:      modelCode,
				Provider:       provider,
				PoolType:       poolType,
				HealthScore:    healthScore,
				HealthStatus:   healthStatus,
				Status:         status,
				Weight:         weight,
				MaxConcurrency: maxConcurrency,
				InstanceCount:  instanceCount,
				CreatedAt:      createdAt.Format(time.RFC3339),
				UpdatedAt:      updatedAt.Format(time.RFC3339),
			})
		}

		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate channels"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleGetChannel returns a single channel with all its instances.
func HandleGetChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel ID"})
			return
		}

		dbCtx := r.Context()

		var (
			id                                   uuid.UUID
			name, poolType, healthStatus, status string
			modelID                              uuid.UUID
			healthScore, weight, maxConcurrency  int
			createdAt, updatedAt                 time.Time
		)
		err = a.Pool.QueryRow(dbCtx,
			`SELECT id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, created_at, updated_at
			 FROM channels WHERE id = $1`, channelID,
		).Scan(&id, &name, &modelID, &poolType, &healthScore, &healthStatus, &status, &weight, &maxConcurrency, &createdAt, &updatedAt)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}

		// Fetch instances
		instRows, err := a.Pool.Query(dbCtx,
			`SELECT id, channel_id, instance_type, base_url, current_load, max_load, status, created_at, updated_at
			 FROM channel_instances WHERE channel_id = $1
			 ORDER BY created_at ASC`, channelID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query instances"})
			return
		}
		defer instRows.Close()

		instances := make([]instanceResponse, 0)
		for instRows.Next() {
			var (
				instID, instChannelID         uuid.UUID
				instType, baseURL, instStatus string
				currentLoad, maxLoad          int
				instCreatedAt, instUpdatedAt  time.Time
			)
			if err := instRows.Scan(&instID, &instChannelID, &instType, &baseURL, &currentLoad, &maxLoad,
				&instStatus, &instCreatedAt, &instUpdatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read instance"})
				return
			}
			instances = append(instances, instanceResponse{
				ID:           instID.String(),
				ChannelID:    instChannelID.String(),
				InstanceType: instType,
				BaseURL:      baseURL,
				CurrentLoad:  currentLoad,
				MaxLoad:      maxLoad,
				Status:       instStatus,
				CreatedAt:    instCreatedAt.Format(time.RFC3339),
				UpdatedAt:    instUpdatedAt.Format(time.RFC3339),
			})
		}
		if err := instRows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate instances"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": channelDetailResponse{
				ID:             id.String(),
				Name:           name,
				ModelID:        modelID.String(),
				PoolType:       poolType,
				HealthScore:    healthScore,
				HealthStatus:   healthStatus,
				Status:         status,
				Weight:         weight,
				MaxConcurrency: maxConcurrency,
				CreatedAt:      createdAt.Format(time.RFC3339),
				UpdatedAt:      updatedAt.Format(time.RFC3339),
				Instances:      instances,
			},
		})
	}
}

// HandleCreateChannel creates a new channel.
func HandleCreateChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req createChannelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Name == "" || req.ModelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and model_id are required"})
			return
		}

		modelID, err := uuid.Parse(req.ModelID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model_id"})
			return
		}

		dbCtx := r.Context()

		// Validate model exists
		var existingModelID uuid.UUID
		err = a.Pool.QueryRow(dbCtx, `SELECT id FROM models WHERE id = $1`, modelID).Scan(&existingModelID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Model not found"})
			return
		}

		// Apply defaults
		poolType := req.PoolType
		if poolType == "" {
			poolType = "shared"
		}
		weight := 100
		if req.Weight != nil {
			weight = *req.Weight
		}
		maxConcurrency := 10
		if req.MaxConcurrency != nil {
			maxConcurrency = *req.MaxConcurrency
		}

		channelID := uuid.New()
		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`INSERT INTO channels (id, name, model_id, pool_type, weight, max_concurrency, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7)`,
			channelID, req.Name, modelID, poolType, weight, maxConcurrency, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create channel"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": channelResponse{
				ID:             channelID.String(),
				Name:           req.Name,
				ModelID:        modelID.String(),
				PoolType:       poolType,
				HealthScore:    100,
				HealthStatus:   "healthy",
				Status:         "active",
				Weight:         weight,
				MaxConcurrency: maxConcurrency,
				InstanceCount:  0,
				CreatedAt:      now.Format(time.RFC3339),
				UpdatedAt:      now.Format(time.RFC3339),
			},
		})
	}
}

// HandleUpdateChannel updates an existing channel.
func HandleUpdateChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel ID"})
			return
		}

		var req updateChannelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		dbCtx := r.Context()

		// Check channel exists and read current values
		var (
			currentName                          string
			currentPoolType                      string
			currentWeight, currentMaxConcurrency int
		)
		err = a.Pool.QueryRow(dbCtx,
			`SELECT name, pool_type, weight, max_concurrency FROM channels WHERE id = $1`, channelID,
		).Scan(&currentName, &currentPoolType, &currentWeight, &currentMaxConcurrency)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}

		// Merge: apply only non-nil fields
		name := currentName
		if req.Name != nil {
			name = *req.Name
		}
		poolType := currentPoolType
		if req.PoolType != nil {
			poolType = *req.PoolType
		}
		weight := currentWeight
		if req.Weight != nil {
			weight = *req.Weight
		}
		maxConcurrency := currentMaxConcurrency
		if req.MaxConcurrency != nil {
			maxConcurrency = *req.MaxConcurrency
		}

		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`UPDATE channels SET name = $1, pool_type = $2, weight = $3, max_concurrency = $4, updated_at = $5 WHERE id = $6`,
			name, poolType, weight, maxConcurrency, now, channelID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update channel"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": channelResponse{
				ID:             channelID.String(),
				Name:           name,
				PoolType:       poolType,
				Weight:         weight,
				MaxConcurrency: maxConcurrency,
				UpdatedAt:      now.Format(time.RFC3339),
			},
		})
	}
}

// HandleDeleteChannel soft-deletes a channel by setting status to 'inactive'.
func HandleDeleteChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel ID"})
			return
		}

		dbCtx := r.Context()

		// Check channel exists
		var existingStatus string
		err = a.Pool.QueryRow(dbCtx, `SELECT status FROM channels WHERE id = $1`, channelID).Scan(&existingStatus)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}

		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`UPDATE channels SET status = 'inactive', updated_at = $1 WHERE id = $2`,
			now, channelID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deactivate channel"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "deleted",
			"id":     channelID.String(),
		})
	}
}

// HandleAddInstance adds an instance to a channel.
func HandleAddInstance(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel ID"})
			return
		}

		var req addInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.InstanceType == "" || req.BaseURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instance_type and base_url are required"})
			return
		}

		maxLoad := 10
		if req.MaxLoad != nil {
			maxLoad = *req.MaxLoad
		}
		if maxLoad <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max_load must be positive"})
			return
		}

		dbCtx := r.Context()

		// Check channel exists
		var existingStatus string
		err = a.Pool.QueryRow(dbCtx, `SELECT status FROM channels WHERE id = $1`, channelID).Scan(&existingStatus)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}

		config := req.Config
		if len(config) == 0 {
			config = json.RawMessage("{}")
		}

		instanceID := uuid.New()
		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, max_load, config, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7)`,
			instanceID, channelID, req.InstanceType, req.BaseURL, maxLoad, config, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create instance"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": instanceResponse{
				ID:           instanceID.String(),
				ChannelID:    channelID.String(),
				InstanceType: req.InstanceType,
				BaseURL:      req.BaseURL,
				CurrentLoad:  0,
				MaxLoad:      maxLoad,
				Status:       "active",
				CreatedAt:    now.Format(time.RFC3339),
				UpdatedAt:    now.Format(time.RFC3339),
			},
		})
	}
}

// HandleRemoveInstance soft-deletes a channel instance by setting status to 'inactive'.
func HandleRemoveInstance(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel ID"})
			return
		}

		instanceID, err := uuid.Parse(chi.URLParam(r, "instanceId"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid instance ID"})
			return
		}

		dbCtx := r.Context()

		// Check channel exists
		var chStatus string
		err = a.Pool.QueryRow(dbCtx, `SELECT status FROM channels WHERE id = $1`, channelID).Scan(&chStatus)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}

		// Check instance exists (but accept already-inactive as idempotent)
		var instStatus string
		err = a.Pool.QueryRow(dbCtx,
			`SELECT status FROM channel_instances WHERE id = $1 AND channel_id = $2`,
			instanceID, channelID,
		).Scan(&instStatus)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Instance not found"})
			return
		}

		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`UPDATE channel_instances SET status = 'inactive', updated_at = $1 WHERE id = $2 AND channel_id = $3`,
			now, instanceID, channelID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove instance"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "removed",
			"id":     instanceID.String(),
		})
	}
}
