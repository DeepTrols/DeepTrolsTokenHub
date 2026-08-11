package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/service/cache"
)

// Config holds all application configuration loaded from environment.
type Config struct {
	DB            DBConfig
	Redis         RedisConfig
	LiteLLM       LiteLLMConfig
	Server        ServerConfig
	JWT           JWTConfig
	Encryption    EncryptionConfig
	Bootstrap     BootstrapConfig
	Cookie        CookieConfig
	ResponseCache ResponseCacheConfig
	// FakePayment enables the demo-only topup faucet and signup bonus.
	// MUST be false in production. When false, the topup endpoint returns 403
	// and no bonus balance is granted.
	FakePayment bool
}

// ResponseCacheConfig tunes the request-response cache.
type ResponseCacheConfig struct {
	// TTLSeconds is the cache entry lifetime. Default 3600 (1 hour).
	TTLSeconds int
	// CacheModels is a comma-separated whitelist of model codes.
	// Empty = all models are cacheable.
	CacheModels string
}

// ToServiceConfig converts the env config to the cache service config.
func (c ResponseCacheConfig) ToServiceConfig() cache.ServiceConfig {
	cfg := cache.ServiceConfig{
		TTL:            time.Duration(c.TTLSeconds) * time.Second,
		AcceptedModels: make(map[string]bool),
	}
	for _, m := range strings.Split(c.CacheModels, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			cfg.AcceptedModels[m] = true
		}
	}
	return cfg
}

type DBConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type LiteLLMConfig struct {
	BaseURL   string
	MasterKey string
}

type ServerConfig struct {
	Port string
	Host string
}

type JWTConfig struct {
	Secret      string
	ExpiryHours int
}

type EncryptionConfig struct {
	Key string
}

type BootstrapConfig struct {
	AdminEmail    string
	AdminPassword string
}

// CookieConfig holds HTTP cookie settings for auth tokens.
type CookieConfig struct {
	Name          string
	Secure        bool
	MaxAgeSeconds int
	SameSite      string
}

// Load reads configuration from environment variables.
// Fails fast (returns an error) on missing required values.
func Load() (*Config, error) {
	// Pre-fetch required values; a missing variable is a returned error,
	// not a panic.
	var dbURL, llmBase, llmKey, jwtSecret, encKey string
	var err error
	if dbURL, err = requireEnv("DATABASE_URL"); err != nil {
		return nil, err
	}
	if llmBase, err = requireEnv("LITELLM_BASE_URL"); err != nil {
		return nil, err
	}
	if llmKey, err = requireEnv("LITELLM_MASTER_KEY"); err != nil {
		return nil, err
	}
	if jwtSecret, err = requireEnv("JWT_SECRET"); err != nil {
		return nil, err
	}
	if encKey, err = requireEnv("ENCRYPTION_KEY"); err != nil {
		return nil, err
	}

	cfg := &Config{
		DB: DBConfig{
			URL: dbURL,
		},
		Redis: RedisConfig{
			URL: os.Getenv("REDIS_URL"),
		},
		LiteLLM: LiteLLMConfig{
			BaseURL:   llmBase,
			MasterKey: llmKey,
		},
		Server: ServerConfig{
			Port: envOrDefault("API_PORT", "8080"),
			Host: envOrDefault("API_HOST", "0.0.0.0"),
		},
		JWT: JWTConfig{
			Secret:      jwtSecret,
			ExpiryHours: envOrDefaultInt("JWT_EXPIRY_HOURS", 24),
		},
		Encryption: EncryptionConfig{
			Key: encKey,
		},
		Bootstrap: BootstrapConfig{
			AdminEmail:    envOrDefault("ADMIN_EMAIL", "deeptrols@admin.com"),
			AdminPassword: envOrDefault("ADMIN_PASSWORD", "deeptrols@2026"),
		},
		ResponseCache: ResponseCacheConfig{
			TTLSeconds:  envOrDefaultInt("CACHE_TTL_SECONDS", 3600),
			CacheModels: os.Getenv("CACHE_MODELS"),
		},
		FakePayment: envOrDefaultBool("ENABLE_FAKE_PAYMENT", false),
	}

	// Cookie config depends on JWT expiry, so we compute it after defaults are set.
	cfg.Cookie = CookieConfig{
		Name:          envOrDefault("COOKIE_NAME", "auth_token"),
		Secure:        envOrDefaultBool("COOKIE_SECURE", true),
		MaxAgeSeconds: cfg.JWT.ExpiryHours * 3600,
		SameSite:      envOrDefault("COOKIE_SAMESITE", "Strict"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes, got %d", len(c.JWT.Secret))
	}
	if len(c.Encryption.Key) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(c.Encryption.Key))
	}

	// Fail-fast guard: known dev-only weak secrets are rejected unless the
	// demo money faucet is explicitly enabled (development mode). This is the
	// last line of defense against shipping the default compose values to prod.
	if !c.FakePayment {
		weakSecrets := []string{
			"change-me-in-production-jwt-secret", // docker-compose default JWT
			"abcdefghijklmnopqrstuvwxyz123456",   // docker-compose default ENCRYPTION_KEY
			"deeptrols@2026",                     // README / compose default admin password
			"sk-litellm-master-dev",              // compose default LiteLLM key
		}
		for _, w := range weakSecrets {
			if c.JWT.Secret == w || c.Encryption.Key == w ||
				c.Bootstrap.AdminPassword == w || c.LiteLLM.MasterKey == w {
				return fmt.Errorf("config: weak default secret %q detected in production mode; set a strong value in .env (or set ENABLE_FAKE_PAYMENT=true for development only)", w)
			}
		}
	}
	return nil
}

func (c ServerConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func requireEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return val, nil
}

func envOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func envOrDefaultInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func envOrDefaultBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1" || val == "yes"
}
