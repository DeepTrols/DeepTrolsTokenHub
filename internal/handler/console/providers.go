package console

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// providerResponse is the JSON shape returned for a single provider in list/detail responses.
type providerResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"`
	BaseURL    string   `json:"base_url"`
	MaskedKey  string   `json:"masked_key"`
	Status     string   `json:"status"`
	ModelCount int      `json:"model_count"`
	ChannelIDs []string `json:"channel_ids"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// createProviderRequest is the request body for creating or updating a provider credential.
type createProviderRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

// updateProviderRequest is the request body for updating a provider credential.
// All fields are optional — only non-empty fields are applied.
type updateProviderRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// defaultBaseURLs maps provider names to their default API base URLs (no trailing version path).
var defaultBaseURLs = map[string]string{
	"openai":      "https://api.openai.com",
	"anthropic":   "https://api.anthropic.com",
	"google":      "https://generativelanguage.googleapis.com",
	"deepseek":    "https://api.deepseek.com",
	"qwen":        "https://dashscope.aliyuncs.com/compatible-mode",
	"zhipu":       "https://open.bigmodel.cn/api/paas/v4",
	"moonshot":    "https://api.moonshot.cn",
	"bytedance":   "https://ark.cn-beijing.volces.com/api/v3",
	"baidu":       "https://qianfan.baidubce.com/v2",
	"xfyun":       "https://spark-api-open.xf-yun.com/v1",
	"tencent":     "https://api.hunyuan.cloud.tencent.com/v1",
	"lingyi":      "https://api.lingyiwanwu.com/v1",
	"openrouter":  "https://openrouter.ai/api",
	"siliconflow": "https://api.siliconflow.cn",
}

// requireAdmin checks that the request context contains a valid user ID and admin role.
// Returns 0 on success, or an HTTP status code (401/403) on failure.
func requireAdmin(r *http.Request) int {
	if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
		return http.StatusUnauthorized
	}
	role, err := jwtutil.RoleFromContext(r.Context())
	if err != nil || role != "admin" {
		return http.StatusForbidden
	}
	return 0
}

// rejectNonAdmin writes an auth error if requireAdmin fails, and returns true if rejected.
func rejectNonAdmin(w http.ResponseWriter, r *http.Request) bool {
	code := requireAdmin(r)
	if code == 0 {
		return false
	}
	msg := "Not authenticated"
	if code == http.StatusForbidden {
		msg = "Admin access required"
	}
	writeJSON(w, code, map[string]string{"error": msg})
	return true
}

// HandleListProviders returns provider credentials grouped by (base_url, api_key).
// Instead of showing every per-model channel separately, it aggregates channels
// that share the same upstream credential into a single entry.
func HandleListProviders(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		// Group channels by (base_url, config->>'api_key') — each unique
		// credential appears once regardless of how many models it covers.
		rows, err := a.Pool.Query(r.Context(),
			`WITH creds AS (
				SELECT
					ci.base_url,
					ci.config->>'api_key' AS api_key,
					ci.config->>'provider' AS provider_type,
					ci.config->>'display_name' AS display_name,
					ch.id AS channel_id,
					ch.name AS channel_name,
					ch.status,
					ci.created_at,
					ci.updated_at,
					ci.config
				FROM channels ch
				LEFT JOIN channel_instances ci ON ci.channel_id = ch.id
				WHERE ch.status = 'active'
			)
			SELECT
				(MIN(channel_id::text))::uuid AS id,
				COALESCE(MAX(display_name), MAX(channel_name)) AS display_name,
				COALESCE(MAX(provider_type), '') AS provider_type,
				COALESCE(base_url, '') AS base_url,
				COALESCE(MAX(api_key), '') AS api_key,
				COUNT(*) AS model_count,
				STRING_AGG(channel_id::text, ',') AS channel_ids_csv,
				COALESCE(MIN(created_at), NOW()) AS created_at,
				COALESCE(MAX(updated_at), NOW()) AS updated_at
			FROM creds
			GROUP BY base_url, api_key
			ORDER BY display_name, base_url`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query providers"})
			return
		}
		defer rows.Close()

		type row struct {
			id            uuid.UUID
			displayName   string
			providerType  string
			baseURL       string
			apiKey        string
			modelCount    int
			channelIDsCSV string
			createdAt     time.Time
			updatedAt     time.Time
		}

		var rows2 []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.displayName, &r.providerType, &r.baseURL, &r.apiKey, &r.modelCount, &r.channelIDsCSV, &r.createdAt, &r.updatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read provider"})
				return
			}
			rows2 = append(rows2, r)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate providers"})
			return
		}

		response := make([]providerResponse, 0, len(rows2))
		for _, r := range rows2 {
			channelIDs := strings.Split(r.channelIDsCSV, ",")
			if r.channelIDsCSV == "" {
				channelIDs = []string{}
			}
			response = append(response, providerResponse{
				ID:         r.id.String(),
				Name:       r.displayName,
				Provider:   r.providerType,
				BaseURL:    r.baseURL,
				MaskedKey:  maskAPIKey(r.apiKey),
				Status:     "active",
				ModelCount: r.modelCount,
				ChannelIDs: channelIDs,
				CreatedAt:  r.createdAt.Format(time.RFC3339),
				UpdatedAt:  r.updatedAt.Format(time.RFC3339),
			})
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleCreateProvider creates a provider, auto-discovers its models, and
// creates channels + instances for every model.
func HandleCreateProvider(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req createProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Name == "" || req.Provider == "" || req.APIKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, provider, and api_key are required"})
			return
		}

		baseURL := strings.TrimRight(req.BaseURL, "/")
		if baseURL == "" {
			if defaultURL, ok := defaultBaseURLs[req.Provider]; ok {
				baseURL = strings.TrimRight(defaultURL, "/")
			}
		}

		dbCtx := r.Context()

		// Auto-discover models from upstream
		discovered, discoverErr := discoverModelsFn(req.Provider, baseURL, req.APIKey)
		if discoverErr != nil {
			log.Printf("provider: model discovery failed for %s: %v", req.Provider, discoverErr)
		}

		now := time.Now().UTC()
		created := 0

		if len(discovered) > 0 {
			for _, mdl := range discovered {
				// Skip models that don't match this provider
				if !matchesProvider(req.Provider, strings.ToLower(mdl.ID)) {
					continue
				}

				modelID := uuid.New()
				channelID := uuid.New()
				instanceID := uuid.New()

				// Determine model code and display name
				code := mdl.ID
				displayName := mdl.ID
				if strings.Contains(code, "/") {
					parts := strings.SplitN(code, "/", 2)
					if len(parts) == 2 {
						code = parts[1]
						displayName = parts[1]
					}
				}

				// Upsert model
				_, err := a.Pool.Exec(dbCtx,
					`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
				 VALUES ($1,$2,$3,'chat',$4,'active','GA',$5,$5)
				 ON CONFLICT (code) DO UPDATE SET display_name=EXCLUDED.display_name, provider=EXCLUDED.provider, updated_at=$5`,
					modelID, code, req.Provider, displayName, now,
				)
				if err != nil {
					log.Printf("provider: upsert model %s: %v", code, err)
					continue
				}

				// Create channel
				_, err = a.Pool.Exec(dbCtx,
					`INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, created_at, updated_at)
				 VALUES ($1,$2,$3,'shared',100,'healthy','active',100,10,$4,$4)
				 ON CONFLICT DO NOTHING`,
					channelID, req.Name+"-"+code, modelID, now,
				)
				if err != nil {
					log.Printf("provider: create channel for %s: %v", code, err)
					continue
				}

				// Create channel instance with display name preserved
				cfg, _ := json.Marshal(map[string]string{"api_key": req.APIKey, "provider": req.Provider, "display_name": req.Name})
				_, err = a.Pool.Exec(dbCtx,
					`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, current_load, max_load, config, status, created_at, updated_at)
				 VALUES ($1,$2,'serverless',$3,$4,0,10,$5,'active',$6,$6)
				 ON CONFLICT DO NOTHING`,
					instanceID, channelID, baseURL, mdl.ID, string(cfg), now,
				)
				if err != nil {
					log.Printf("provider: create instance for %s: %v", code, err)
					continue
				}

				// Add default pricing
				_, _ = a.Pool.Exec(dbCtx,
					`INSERT INTO model_pricing (id, model_id, tenant_id, request_type, pricing_dimension, unit_name, unit_price, upstream_cost, currency, created_at, updated_at)
				 VALUES (uuid_generate_v4(),$1,NULL,'chat','input','1K tokens','0.001','0.001','CNY',$2,$2)
				 ON CONFLICT DO NOTHING`,
					modelID, now,
				)
				_, _ = a.Pool.Exec(dbCtx,
					`INSERT INTO model_pricing (id, model_id, tenant_id, request_type, pricing_dimension, unit_name, unit_price, upstream_cost, currency, created_at, updated_at)
				 VALUES (uuid_generate_v4(),$1,NULL,'chat','output','1K tokens','0.001','0.001','CNY',$2,$2)
				 ON CONFLICT DO NOTHING`,
					modelID, now,
				)

				created++
			}
		}

		// If discovery failed, save a placeholder so the credential appears in the UI.
		if discoverErr != nil && created == 0 {
			mid := uuid.New()
			cid := uuid.New()
			iid := uuid.New()
			placeholderCode := req.Provider + "-pending"
			a.Pool.Exec(dbCtx,
				`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
				 VALUES ($1,$2,$3,'chat',$4,'active','GA',$5,$5) ON CONFLICT (code) DO NOTHING`,
				mid, placeholderCode, req.Provider, req.Name+" (待配置)", now,
			)
			a.Pool.Exec(dbCtx,
				`INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, created_at, updated_at)
				 VALUES ($1,$2,$3,'shared',100,'healthy','active',100,10,$4,$4) ON CONFLICT DO NOTHING`,
				cid, req.Name+"-pending", mid, now,
			)
			cfg, _ := json.Marshal(map[string]string{"api_key": req.APIKey, "provider": req.Provider, "display_name": req.Name})
			a.Pool.Exec(dbCtx,
				`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, current_load, max_load, config, status, created_at, updated_at)
				 VALUES ($1,$2,'serverless',$3,$4,0,10,$5,'active',$6,$6) ON CONFLICT DO NOTHING`,
				iid, cid, baseURL, placeholderCode, string(cfg), now,
			)
			created = 1
		}

		resp := map[string]interface{}{
			"provider":       req.Provider,
			"name":           req.Name,
			"base_url":       baseURL,
			"models_created": created,
			"total_models":   len(discovered),
			"status":         "active",
		}
		if discoverErr != nil {
			resp["warning"] = "模型自动发现失败: " + discoverErr.Error() + "。凭证已保存，请检查 API Key。"
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

// discoverModelsFn is the model-discovery entry point used by the create/update
// handlers. It is a variable so tests can stub it and avoid external network
// calls; production uses discoverModels.
var discoverModelsFn = discoverModels

// discoverModels calls the upstream provider's /models endpoint and returns
// a list of discovered model IDs. Supports OpenAI-compatible, Anthropic, and Google APIs.
func discoverModels(provider, baseURL, apiKey string) ([]modelRef, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Build the models endpoint URL based on provider type.
	endpoints := buildModelEndpoints(provider, baseURL, apiKey)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	var lastErr error
	for _, ep := range endpoints {
		req, err := http.NewRequest("GET", ep.url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		for k, v := range ep.headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		// Parse response according to provider format.
		models, err := ep.parse(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}
		if len(models) > 0 {
			return models, nil
		}
		lastErr = fmt.Errorf("empty model list")
	}
	return nil, lastErr
}

// modelEndpoint describes a single models-list endpoint attempt.
type modelEndpoint struct {
	url     string
	headers map[string]string
	parse   func(r io.Reader) ([]modelRef, error)
}

// buildModelEndpoints constructs one or more endpoint attempts for a provider.
func buildModelEndpoints(provider, baseURL, apiKey string) []modelEndpoint {
	baseURL = strings.TrimRight(baseURL, "/")

	switch provider {
	case "google":
		// Google Gemini: key is a query parameter, response is {models: [{name: "models/gemini-2.0-flash"}]}
		u := baseURL + "/v1beta/models?key=" + url.QueryEscape(apiKey)
		return []modelEndpoint{{
			url:     u,
			headers: map[string]string{},
			parse:   parseGoogleModels,
		}}

	case "anthropic":
		// Anthropic: x-api-key header, returns {data: [{id: "claude-sonnet-4-20250514"}]}
		return []modelEndpoint{{
			url:     baseURL + "/v1/models",
			headers: map[string]string{"x-api-key": apiKey},
			parse:   parseOpenAIModels,
		}}

	default:
		// OpenAI-compatible (OpenAI, DeepSeek, Qwen, etc.):
		// Authorization: Bearer <key>, returns {data: [{id: "gpt-4o"}]}
		var eps []modelEndpoint
		headers := map[string]string{"Authorization": "Bearer " + apiKey}

		// Try models endpoint under /v1 first, then bare path.
		if !strings.HasSuffix(baseURL, "/v1") {
			eps = append(eps, modelEndpoint{
				url:     baseURL + "/v1/models",
				headers: headers,
				parse:   parseOpenAIModels,
			})
		}
		eps = append(eps, modelEndpoint{
			url:     baseURL + "/models",
			headers: headers,
			parse:   parseOpenAIModels,
		})
		return eps
	}
}

// parseOpenAIModels parses OpenAI-compatible {data: [{id: "..."}]} responses.
func parseOpenAIModels(r io.Reader) ([]modelRef, error) {
	var result struct {
		Data []modelRef `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// parseGoogleModels parses Google Gemini {models: [{name: "models/..."}]} responses.
func parseGoogleModels(r io.Reader) ([]modelRef, error) {
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return nil, err
	}
	refs := make([]modelRef, 0, len(result.Models))
	for _, m := range result.Models {
		// Strip "models/" prefix to get the model ID.
		id := strings.TrimPrefix(m.Name, "models/")
		refs = append(refs, modelRef{ID: id})
	}
	return refs, nil
}

// HandleSyncProviderModels re-discovers models from a provider's upstream API
// and creates/updates models, channels, and instances.
func HandleSyncProviderModels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		providerID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider ID"})
			return
		}

		dbCtx := r.Context()

		// Load provider config from channel_instances
		var pv, baseURL, apiKey, displayName string
		err = a.Pool.QueryRow(dbCtx,
			`SELECT ci.config->>'provider', ci.base_url, ci.config->>'api_key', ci.config->>'display_name'
			 FROM channel_instances ci
			 JOIN channels ch ON ci.channel_id = ch.id
			 WHERE ch.id = $1 AND ci.status = 'active'
			 LIMIT 1`, providerID).Scan(&pv, &baseURL, &apiKey, &displayName)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Provider not found"})
			return
		}

		if baseURL == "" {
			if url, ok := defaultBaseURLs[pv]; ok {
				baseURL = url
			}
		}

		discovered, discoverErr := discoverModelsFn(pv, baseURL, apiKey)
		if discoverErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Model discovery failed: " + discoverErr.Error()})
			return
		}

		now := time.Now().UTC()
		created := 0
		for _, mdl := range discovered {
			if !matchesProvider(pv, strings.ToLower(mdl.ID)) {
				continue
			}
			code := mdl.ID
			if strings.Contains(code, "/") {
				parts := strings.SplitN(code, "/", 2)
				if len(parts) == 2 {
					code = parts[1]
				}
			}
			modelID := uuid.New()
			_, err := a.Pool.Exec(dbCtx,
				`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
				 VALUES ($1,$2,$3,'chat',$4,'active','GA',$5,$5)
				 ON CONFLICT (code) DO UPDATE SET provider=EXCLUDED.provider, display_name=EXCLUDED.display_name, updated_at=$5`,
				modelID, code, pv, code, now,
			)
			if err != nil {
				continue
			}
			created++
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"synced":  created,
			"total":   len(discovered),
			"message": fmt.Sprintf("Synced %d models from %s", created, pv),
		})
	}
}

type modelRef struct {
	ID string `json:"id"`
}

// matchesProvider checks whether a model ID belongs to the given provider.
// For known providers, matches common model name patterns. For unknown providers, accepts all.
func matchesProvider(provider, lowerID string) bool {
	switch provider {
	case "deepseek":
		return strings.Contains(lowerID, "deepseek")
	case "qwen":
		return strings.Contains(lowerID, "qwen") || strings.Contains(lowerID, "tongyi")
	case "openai":
		return strings.Contains(lowerID, "gpt") || strings.Contains(lowerID, "o1") ||
			strings.Contains(lowerID, "o3") || strings.Contains(lowerID, "o4") ||
			strings.Contains(lowerID, "dall-e") || strings.Contains(lowerID, "tts") ||
			strings.Contains(lowerID, "whisper")
	case "anthropic":
		return strings.Contains(lowerID, "claude")
	case "google":
		return strings.Contains(lowerID, "gemini")
	default:
		return true // unknown provider: accept all
	}
}

// HandleUpdateProvider updates all channels and instances that share the same
// upstream credential (base_url + api_key).  The URL {id} is any channel ID
// belonging to the credential group.
func HandleUpdateProvider(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		representativeID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider ID"})
			return
		}

		var req updateProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		// No fields to update
		if req.Name == "" && req.BaseURL == "" && req.APIKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "At least one field (name, base_url, api_key) must be provided"})
			return
		}

		dbCtx := r.Context()

		// Look up the credential that this channel belongs to.
		var baseURL, apiKey string
		err = a.Pool.QueryRow(dbCtx,
			`SELECT ci.base_url, ci.config->>'api_key'
			 FROM channel_instances ci
			 WHERE ci.channel_id = $1
			 LIMIT 1`, representativeID,
		).Scan(&baseURL, &apiKey)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Provider credential not found"})
			return
		}

		now := time.Now().UTC()

		tx, err := a.Pool.Begin(dbCtx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
			return
		}
		defer tx.Rollback(dbCtx)

		if req.Name != "" {
			_, err = tx.Exec(dbCtx,
				`UPDATE channels
				 SET name = $1, updated_at = $2
				 WHERE id IN (
				   SELECT ci2.channel_id
				   FROM channel_instances ci2
				   WHERE ci2.base_url = $3 AND ci2.config->>'api_key' = $4
				 )`,
				req.Name, now, baseURL, apiKey,
			)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update channel names"})
				return
			}
		}

		if req.BaseURL != "" {
			_, err = tx.Exec(dbCtx,
				`UPDATE channel_instances
				 SET base_url = $1, updated_at = $2
				 WHERE base_url = $3 AND config->>'api_key' = $4`,
				req.BaseURL, now, baseURL, apiKey,
			)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update base URL"})
				return
			}
		}

		if req.APIKey != "" {
			// Build updated config: merge new API key into each instance's config.
			_, err = tx.Exec(dbCtx,
				`UPDATE channel_instances
				 SET config = jsonb_set(
				       COALESCE(config, '{}'::jsonb),
				       '{api_key}',
				       to_jsonb($1::text)
				     ),
				     updated_at = $2
				 WHERE base_url = $3 AND config->>'api_key' = $4`,
				req.APIKey, now, baseURL, apiKey,
			)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update API key"})
				return
			}
		}

		if err := tx.Commit(dbCtx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "updated",
			"id":     representativeID.String(),
		})
	}
}

// HandleDeleteProvider soft-deletes all channels and instances that share the
// same upstream credential (base_url + api_key).  The URL {id} is any channel ID
// belonging to the credential group.
func HandleDeleteProvider(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		representativeID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider ID"})
			return
		}

		dbCtx := r.Context()

		// Look up the credential that this channel belongs to.
		var baseURL, apiKey string
		err = a.Pool.QueryRow(dbCtx,
			`SELECT ci.base_url, ci.config->>'api_key'
			 FROM channel_instances ci
			 WHERE ci.channel_id = $1
			 LIMIT 1`, representativeID,
		).Scan(&baseURL, &apiKey)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Provider credential not found"})
			return
		}

		now := time.Now().UTC()
		tx, err := a.Pool.Begin(dbCtx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
			return
		}
		defer tx.Rollback(dbCtx)

		// Soft-delete all channels sharing this credential.
		_, err = tx.Exec(dbCtx,
			`UPDATE channels
			 SET status = 'inactive', updated_at = $1
			 WHERE id IN (
			   SELECT ci2.channel_id
			   FROM channel_instances ci2
			   WHERE ci2.base_url = $2 AND ci2.config->>'api_key' = $3
			 )`,
			now, baseURL, apiKey,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deactivate channels"})
			return
		}

		// Soft-delete all instances sharing this credential.
		result, err := tx.Exec(dbCtx,
			`UPDATE channel_instances
			 SET status = 'inactive', updated_at = $1
			 WHERE base_url = $2 AND config->>'api_key' = $3`,
			now, baseURL, apiKey,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deactivate instances"})
			return
		}

		if err := tx.Commit(dbCtx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
			return
		}

		deleted := result.RowsAffected()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         "deleted",
			"credential_id":  representativeID.String(),
			"deleted_models": deleted,
		})
	}
}

// maskAPIKey returns a masked version of an API key, showing only the last 4 characters.
// For keys shorter than 4 characters, all characters are shown after "****".
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return fmt.Sprintf("****%s", key)
	}
	return fmt.Sprintf("****%s", key[len(key)-4:])
}
