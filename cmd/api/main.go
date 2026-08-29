package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/console"
	"github.com/deeptrols/api/internal/handler/gateway"
	"github.com/deeptrols/api/internal/handler/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create the application with dependency injection.
	application, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}
	defer application.Shutdown()

	// Ensure admin user exists with hardcoded credentials.
	ensureAdminUser(application, cfg)

	// Ensure the system administrator owns the platform tenant so enterprise
	// settings and team management are reachable for role=admin users.
	// Non-fatal: a failure only degrades admin enterprise features, not serving.
	if err := application.EnsurePlatformTenant(context.Background(), uuid.Nil); err != nil {
		log.Printf("ensurePlatformTenant: %v", err)
	}

	r := chi.NewRouter()

	// Security headers (must be first).
	r.Use(middleware.SecurityHeaders())

	// CORS - use explicit origin instead of wildcard when credentials are enabled.
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:5173"
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{corsOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(middleware.RequestLogger(application.Slog))

	// Register app-level routes (health, etc.).
	application.RegisterRoutes(r)

	// Admin-uploaded brand assets (logo/favicon) are served from the local
	// upload dir; production can front this with CDN/object storage.
	uploadDir := application.Config.UploadDir
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))

	// Public, unauthenticated stats for the login page (IP rate-limited).
	r.Get("/api/public/stats",
		middleware.IPRateLimit(application.RateLimiter, 60, 1*time.Minute)(app.PublicStatsHandler(application)).ServeHTTP)

	// Public, unauthenticated site/brand info for the login page and frontend boot.
	r.Get("/api/public/site", console.HandlePublicSite(application))

	// Public model/vendor rankings (new-api rankings parity).
	r.Get("/api/public/rankings", console.HandleRankings(application))

	// Public model catalog + pricing for the /pricing page (new-api parity).
	r.Get("/api/public/pricing", console.HandlePublicPricing(application))

	// GitHub OAuth login (new-api oauth parity).
	r.Get("/api/oauth/github/authorize",
		middleware.IPRateLimit(application.RateLimiter, 10, 1*time.Minute)(console.HandleOAuthAuthorize(application)).ServeHTTP)
	r.Get("/api/oauth/github/callback",
		middleware.IPRateLimit(application.RateLimiter, 30, 1*time.Minute)(console.HandleOAuthCallback(application)).ServeHTTP)
	r.Get("/api/oauth/wechat/authorize",
		middleware.IPRateLimit(application.RateLimiter, 10, 1*time.Minute)(console.HandleWechatAuthorize(application)).ServeHTTP)
	r.Get("/api/oauth/wechat/callback",
		middleware.IPRateLimit(application.RateLimiter, 30, 1*time.Minute)(console.HandleWechatCallback(application)).ServeHTTP)
	r.Get("/api/oauth/google/authorize",
		middleware.IPRateLimit(application.RateLimiter, 10, 1*time.Minute)(console.HandleGoogleAuthorize(application)).ServeHTTP)
	r.Get("/api/oauth/google/callback",
		middleware.IPRateLimit(application.RateLimiter, 30, 1*time.Minute)(console.HandleGoogleCallback(application)).ServeHTTP)

	// Payment gateway callback (public, no auth; verified by signature).
	r.Post("/api/payment/notify/epay", console.HandlePaymentNotify(application))
	r.Get("/api/payment/notify/epay", console.HandlePaymentNotify(application))

	// OpenAI-compatible gateway (public).
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.GatewayAuth(application))
		r.Use(middleware.GatewayRateLimit(application.RateLimiter, 100, 1*time.Minute))
		// 网关响应一律禁止缓存，避免浏览器/代理把旧的模型列表或结果当新数据展示。
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/models", gateway.HandleListModels(application))
		r.Get("/models/{model}", gateway.HandleRetrieveModel(application))
		r.Post("/chat/completions", gateway.HandleChatCompletions(application))
		r.Post("/completions", gateway.HandleCompletions(application))
		r.Post("/responses", gateway.HandleResponses(application))
		r.Post("/messages", gateway.HandleClaudeMessages(application))
		r.Post("/messages/count_tokens", gateway.HandleCountTokens(application))
		r.Post("/embeddings", gateway.HandleEmbeddings(application))
		r.Post("/images/generations", gateway.HandleImagesGenerations(application))
		r.Post("/images/edits", gateway.HandleImagesEdits(application))
		r.Post("/audio/speech", gateway.HandleAudioSpeech(application))
		r.Post("/audio/transcriptions", gateway.HandleAudioTranscriptions(application))
		r.Post("/videos/generations", gateway.HandleVideoGenerations(application))
		r.Get("/videos/generations/{id}", gateway.HandleVideoJobStatus(application))
		r.Delete("/videos/generations/{id}", gateway.HandleVideoJobCancel(application))
	})

	// Console API (JWT-protected, user-facing).
	r.Route("/api/console", func(r chi.Router) {
		r.Post("/auth/login", middleware.LoginRateLimit(application.RateLimiter, 5, 1*time.Minute)(console.HandleLogin(application)).ServeHTTP)
		r.Post("/auth/register", middleware.LoginRateLimit(application.RateLimiter, 5, 1*time.Minute)(console.HandleRegister(application)).ServeHTTP)

		r.Group(func(r chi.Router) {
			r.Use(middleware.ConsoleAuth(application))
			r.Post("/auth/logout", console.HandleLogout(application))
			r.Get("/me", console.HandleMe(application))
			r.Put("/me/profile", console.HandleUpdateProfile(application))
			r.Put("/me/password", console.HandleChangePassword(application))
			r.Get("/profile", console.HandleGetProfile(application))

			r.Get("/api-keys", console.HandleListAPIKeys(application))
			r.Post("/api-keys", console.HandleCreateAPIKey(application))
			r.Put("/api-keys/{id}", console.HandleUpdateAPIKey(application))
			r.Delete("/api-keys/{id}", console.HandleDeleteAPIKey(application))
			r.Get("/api-keys/{id}/secret", console.HandleGetAPIKeySecret(application))
			r.Get("/usage", console.HandleListUsage(application))
			r.Get("/usage/{id}/charge-lines", console.HandleGetUsageChargeLines(application))
			r.Get("/wallet", console.HandleGetWallet(application))
			r.Get("/wallet/transactions", console.HandleListTransactions(application))
			r.Get("/wallet/alert", console.HandleGetBalanceAlert(application))
			r.Put("/wallet/alert", console.HandleSetBalanceAlert(application))
			r.Post("/wallet/topup", console.HandleTopUp(application))
			r.Get("/billing/statement", console.HandleMonthlyStatement(application))
			r.Get("/sessions", console.HandleListSessions(application))
			r.Delete("/sessions", console.HandleRevokeOtherSessions(application))
			r.Delete("/sessions/{id}", console.HandleRevokeSession(application))
			r.Get("/payment/methods", console.HandlePaymentMethods(application))
			r.Post("/payment/order", console.HandleCreatePaymentOrder(application))
			r.Get("/payment/orders", console.HandleListMyPaymentOrders(application))
			r.Post("/redemption/redeem", console.HandleRedeem(application))
			r.Post("/checkin", console.HandleCheckIn(application))
			r.Get("/checkin/status", console.HandleCheckInStatus(application))
			r.Get("/chat/presets", console.HandleChatPresets(application))
			r.Get("/subscription/plans", console.HandleListPlans(application))
			r.Post("/subscription/purchase", console.HandlePurchaseSubscription(application))
			r.Post("/subscription/auto-renew", console.HandleSetAutoRenew(application))
			r.Post("/subscription/order", console.HandleCreateSubscriptionOrder(application))
			r.Get("/subscription/self", console.HandleMySubscriptions(application))
			r.Get("/invite", console.HandleInviteInfo(application))
			// User portal: read-only model listing only
			r.Get("/models", console.HandleListModels(application))
			r.Get("/models/{id}", console.HandleGetModel(application))
			r.Get("/security/login-history", console.HandleLoginHistory(application))
		})
	})

	// Admin API (requires JWT + admin role).
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.ConsoleAuth(application))
		r.Use(middleware.AdminAuth())
		r.Use(middleware.AdminRateLimit(application.RateLimiter, 120, 1*time.Minute))
		r.Use(middleware.AuditAdminWrite(application.Pool))

		r.Get("/models", console.HandleListModels(application))
		r.Post("/models", console.HandleCreateModel(application))
		r.Get("/models/{id}", console.HandleGetModel(application))
		r.Put("/models/{id}", console.HandleUpdateModel(application))
		r.Delete("/models/{id}", console.HandleDeleteModel(application))
		r.Get("/models/catalog/preview", console.HandleCatalogPreview(application))
		r.Post("/models/sync_catalog", console.HandleCatalogSync(application))

		r.Get("/providers", console.HandleListProviders(application))
		r.Post("/providers", console.HandleCreateProvider(application))
		r.Put("/providers/{id}", console.HandleUpdateProvider(application))
		r.Delete("/providers/{id}", console.HandleDeleteProvider(application))
		r.Post("/providers/{id}/sync", console.HandleSyncProviderModels(application))
		r.Post("/providers/{id}/test", console.HandleTestProvider(application))
		r.Get("/providers/{id}/models/preview", console.HandleProviderModelsPreview(application))
		r.Put("/providers/{id}/status", console.HandleSetProviderStatus(application))

		r.Get("/channels", console.HandleListChannels(application))
		r.Post("/channels", console.HandleCreateChannel(application))
		r.Get("/channels/{id}", console.HandleGetChannel(application))
		r.Put("/channels/{id}", console.HandleUpdateChannel(application))
		r.Delete("/channels/{id}", console.HandleDeleteChannel(application))
		r.Put("/channels/{id}/status", console.HandleSetChannelStatus(application))
		r.Post("/channels/test-all", console.HandleBatchTestChannels(application))
		r.Post("/channels/{id}/instances", console.HandleAddInstance(application))
		r.Delete("/channels/{id}/instances/{instanceId}", console.HandleRemoveInstance(application))
		r.Post("/channels/{id}/test", console.HandleChannelTest(application))
		r.Get("/channels/{id}/models", console.HandleChannelModelsList(application))
		r.Get("/channels/{id}/models/preview", console.HandleChannelModelsPreview(application))
		r.Post("/channels/{id}/models/sync", console.HandleChannelModelsSync(application))
		r.Put("/channels/{id}/models/{modelId}", console.HandleSetChannelModelEnabled(application))

		r.Get("/tenants", console.HandleListTenants(application))
		r.Post("/tenants", console.HandleCreateTenant(application))
		r.Get("/tenants/{id}", console.HandleGetTenant(application))
		r.Put("/tenants/{id}", console.HandleUpdateTenant(application))
		r.Delete("/tenants/{id}", console.HandleDeleteTenant(application))

		r.Get("/reconciliation", console.HandleListReconciliationRuns(application))
		r.Get("/reconciliation/summary", console.HandleReconciliationSummary(application))

		r.Get("/users", console.HandleListUsers(application))
		r.Get("/ledger", console.HandleUserLedger(application))
		r.Put("/users/{id}/status", console.HandleUpdateUserStatus(application))
		r.Put("/users/{id}/role", console.HandleUpdateUserRole(application))
		r.Post("/users", console.HandleCreateUser(application))
		r.Delete("/users/{id}", console.HandleDeleteUser(application))

		r.Get("/audit", console.HandleListAuditLogs(application))
		r.Get("/settings/site", console.HandleGetSiteSettings(application))
		r.Put("/settings/site", console.HandleUpdateSiteSettings(application))
		r.Post("/settings/upload", console.HandleUploadLogo(application))
		r.Get("/payment/orders", console.HandleListAllPaymentOrders(application))
		r.Post("/payment/orders/{id}/complete", console.HandleAdminCompletePaymentOrder(application))
		r.Get("/system/info", console.HandleSystemInfo(application))
		r.Post("/redemption", console.HandleCreateRedemption(application))
		r.Get("/redemption", console.HandleListRedemptions(application))
		r.Get("/subscription-plans", console.HandleListSubscriptionPlans(application))
		r.Post("/subscription-plans", console.HandleCreateSubscriptionPlan(application))
		r.Put("/subscription-plans/{id}", console.HandleUpdateSubscriptionPlan(application))
		r.Delete("/subscription-plans/{id}", console.HandleDeleteSubscriptionPlan(application))
		r.Get("/subscriptions", console.HandleListAllSubscriptions(application))
		r.Post("/subscriptions/{id}/cancel", console.HandleCancelSubscription(application))
		r.Get("/gateway/health", console.HandleGatewayHealth(application))
	})

	srv := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           r,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second, // fail slowloris quickly
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB request header cap
	}

	// Graceful shutdown with pool cleanup.
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Println("shutting down server...")
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	fmt.Printf("API server listening on %s\n", cfg.Server.Address())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// ensureAdminUser guarantees the bootstrap admin user exists in the database
// with a proper bcrypt password hash. Hardcoded credentials:
//
//	Email:    deeptrols@admin.com
//	Password: deeptrols@2026
func ensureAdminUser(application *app.App, cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminID := uuid.Nil
	email := cfg.Bootstrap.AdminEmail
	password := cfg.Bootstrap.AdminPassword

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Printf("ensureAdminUser: failed to hash admin password: %v", err)
		return
	}

	// Check if admin already exists.
	existing, err := application.Users.FindByID(ctx, adminID)
	if err == nil {
		// Admin exists — NEVER overwrite the password on restart: a rotated
		// admin password must survive redeploys. Only repair a missing hash.
		if existing.PasswordHash == "" {
			_, execErr := application.Pool.Exec(ctx,
				`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`,
				string(hash), adminID,
			)
			if execErr != nil {
				log.Printf("ensureAdminUser: failed to repair empty admin password: %v", execErr)
			} else {
				log.Printf("ensureAdminUser: repaired empty admin password hash")
			}
		}
		return
	}

	// Admin does not exist — create it.
	now := time.Now().UTC()
	u := &domain.User{
		ID:           adminID,
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  "Administrator",
		Role:         "admin",
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if createErr := application.Users.Create(ctx, u); createErr != nil {
		log.Printf("ensureAdminUser: failed to create admin user: %v", createErr)
		return
	}

	// Also create a wallet for the admin user. Bonus balance is granted only
	// when the demo money faucet is enabled (ENABLE_FAKE_PAYMENT=true).
	bonus := "0"
	if cfg.FakePayment {
		bonus = "10000"
	}
	application.Pool.Exec(ctx,
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, '0', 'CNY', 0, $4, $4)
		 ON CONFLICT DO NOTHING`,
		uuid.New(), adminID, bonus, now,
	)

	log.Printf("ensureAdminUser: admin user created (%s)", email)
}
