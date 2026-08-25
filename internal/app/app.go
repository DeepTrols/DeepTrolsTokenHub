package app

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/deeptrols/api/internal/billing_sync"
	billingadapters "github.com/deeptrols/api/internal/billing_sync/adapters"
	billingpersistence "github.com/deeptrols/api/internal/billing_sync/persistence"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/guardrails"
	guardrailpersistence "github.com/deeptrols/api/internal/guardrails/persistence"
	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/deeptrols/api/internal/pkg/minutebucket"
	"github.com/deeptrols/api/internal/pkg/ratelimit"
	"github.com/deeptrols/api/internal/pkg/redis"
	"github.com/deeptrols/api/internal/provider"
	"github.com/deeptrols/api/internal/repository/apikey"
	"github.com/deeptrols/api/internal/repository/budget"
	"github.com/deeptrols/api/internal/repository/channel"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/deeptrols/api/internal/service/billing"
	"github.com/deeptrols/api/internal/service/cache"
	"github.com/deeptrols/api/internal/service/gateway"
)

// App holds all shared dependencies for the API server.
type App struct {
	Pool    *pgxpool.Pool
	Config  *config.Config
	Healthy bool
	// Slog is the process logger (structured JSON by default).
	Slog *slog.Logger

	// Redis client (nil when Redis is not configured or unavailable).
	Redis *goredis.Client
	// RateLimiter used by login/gateway middleware. When Redis is available
	// this is a Redis-backed limiter with an in-memory fallback; otherwise it
	// is in-memory only.
	RateLimiter ratelimit.RateLimiter

	// Repositories
	APIKeys     apikey.Repository
	Models      model.Repository
	Tenants     tenant.Repository
	Usage       usage.Repository
	Users       user.Repository
	Wallets     wallet.Repository
	Channels    channel.Repository
	Memberships membership.Repository
	Budgets     budget.Repository

	// Services
	Charger *billing.Charger
	Logger  *billing.Logger
	Pricer  *billing.Pricer
	Router  *gateway.Router
	// BillingSync synchronizes external provider billing (OneAPI/NewAPI/Aliyun)
	// into billing_records for reconciliation L3.
	BillingSync     *billingsync.Service
	BillingSyncRepo billingsync.Repository
	// Guardrails evaluates outbound content policies before upstream calls.
	Guardrails         *guardrails.Engine
	GuardrailsPolicies guardrails.PolicySource
	// LoadTracker tracks per-instance in-flight counts in Redis. It is always
	// non-nil; when Redis is unavailable it is a no-op tracker and routing
	// falls back to the DB current_load column.
	LoadTracker *gateway.LoadTracker

	// Executor for chat completions (injected for testability).
	Executor gateway.Executor
	// HttpClient for streaming requests (injected for testability).
	HttpClient *http.Client

	// ResponseCache caches identical upstream responses to skip billing.
	ResponseCache *cache.Service
	// MinuteBuckets enforces per-API-key RPM/TPM limits (Redis primary,
	// PostgreSQL fallback). Nil disables the check.
	MinuteBuckets minutebucket.Store
}

// NewApp creates a new App instance with a database connection pool
// and all wired dependencies.
func NewApp(cfg *config.Config) (*App, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		return nil, fmt.Errorf("app: failed to create database pool: %w", err)
	}

	a := &App{
		Pool:    pool,
		Config:  cfg,
		Healthy: true,
		Slog:    NewSlogLogger(cfg),
	}

	// Wire repositories.
	a.APIKeys = apikey.NewPostgresRepository(pool)
	a.Models = model.NewPostgresRepository(pool)
	a.Tenants = tenant.NewPostgresRepository(pool)
	a.Usage = usage.NewPostgresRepository(pool)
	a.Users = user.NewPostgresRepository(pool)
	a.Wallets = wallet.NewPostgresRepository(pool)
	a.Channels = channel.NewPostgresRepository(pool)
	a.Memberships = membership.NewPostgresRepository(pool)
	a.Budgets = budget.NewPostgresRepository(pool)
	billingSyncRepo := billingpersistence.NewPostgresRepository(pool, []byte(cfg.Encryption.Key))
	a.BillingSyncRepo = billingSyncRepo
	a.BillingSync = billingsync.NewService(billingSyncRepo,
		billingadapters.NewRegistry(&http.Client{Timeout: 30 * time.Second}))
	guardrailRepo := guardrailpersistence.NewPostgresRepository(pool)
	a.GuardrailsPolicies = guardrailRepo
	a.Guardrails = guardrails.NewEngine(nil)

	// Wire services.
	a.Charger = billing.NewCharger(a.Wallets)
	a.Logger = billing.NewLoggerWithPool(a.Usage, a.Pool)
	a.Pricer = billing.NewPricer(a.Models.(model.PricingRepository))
	a.Router = gateway.NewRouter(a.Models, a.Channels)
	a.Executor = provider.NewOpenAICompatAdapter()
	a.HttpClient = &http.Client{Timeout: 120 * time.Second}

	// Wire rate limiter: Redis-backed with in-memory fallback when Redis is
	// configured and reachable; in-memory only otherwise.
	a.initRateLimiter(ctx, cfg)

	// Wire response cache — MUST run after initRateLimiter so a.Redis is set.
	// If Redis is unavailable, caching is silently disabled (no-op).
	a.ResponseCache = cache.New(a.Redis, cfg.ResponseCache.ToServiceConfig())

	// Wire per-key minute quota buckets after Redis is known.
	a.MinuteBuckets = minutebucket.NewStore(a.Redis, minutebucket.NewPostgresStore(pool))

	// Wire real-time load tracking — MUST run after initRateLimiter so
	// a.Redis is set. Without Redis the tracker is a no-op and routing falls
	// back to the database current_load column.
	a.LoadTracker = gateway.NewLoadTracker(a.Redis, time.Duration(cfg.LoadTTLSeconds)*time.Second)
	a.Router.SetLoadSource(a.LoadTracker)

	return a, nil
}

// initRateLimiter sets a.Redis and a.RateLimiter. A Redis outage at startup is
// non-fatal: we log and fall back to the in-memory limiter so the API still
// serves traffic.
func (a *App) initRateLimiter(ctx context.Context, cfg *config.Config) {
	memoryLimiter := ratelimit.NewMemoryRateLimiter()

	if cfg.Redis.URL == "" {
		a.RateLimiter = memoryLimiter
		return
	}

	client, err := redis.NewClient(ctx, cfg.Redis.URL)
	if err != nil {
		log.Printf("app: redis unavailable; using in-memory rate limiter")
		a.RateLimiter = memoryLimiter
		return
	}

	a.Redis = client
	a.RateLimiter = ratelimit.NewFallbackRateLimiter(
		ratelimit.NewRedisRateLimiter(client),
		memoryLimiter,
	)
	log.Printf("app: rate limiter backed by Redis with in-memory fallback")
}

// RegisterRoutes wires all HTTP endpoints onto the given chi router.
func (a *App) RegisterRoutes(r chi.Router) {
	r.Get("/health", healthHandler)
	r.Get("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler(a))
}

// Shutdown gracefully releases resources held by the App.
func (a *App) Shutdown() {
	if a.Pool != nil {
		a.Pool.Close()
	}
	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			log.Printf("app: redis close error: %v", err)
		}
	}
}
