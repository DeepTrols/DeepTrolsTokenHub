package config

import (
	"os"
	"strings"
	"testing"
)

// TestMain runs the config test suite in development mode so the existing
// tests (which intentionally use short/dev secrets) keep exercising the
// length/format validations. The production weak-secret guard itself is
// covered separately by TestConfig_WeakSecretsRejectedInProduction.
func TestMain(m *testing.M) {
	os.Setenv("ENABLE_FAKE_PAYMENT", "true")
	code := m.Run()
	os.Unsetenv("ENABLE_FAKE_PAYMENT")
	os.Exit(code)
}

// TestConfig_WeakSecretsRejectedInProduction verifies the fail-fast guard:
// known dev-only defaults must be rejected when ENABLE_FAKE_PAYMENT is false
// (production), and accepted when it is true (development).
func TestConfig_WeakSecretsRejectedInProduction(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "sk-litellm-master-dev")
	os.Setenv("JWT_SECRET", "change-me-in-production-jwt-secret")
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	os.Setenv("ADMIN_EMAIL", "admin@test.com")
	os.Setenv("ADMIN_PASSWORD", "deeptrols@2026")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ENCRYPTION_KEY")
		os.Unsetenv("ADMIN_EMAIL")
		os.Unsetenv("ADMIN_PASSWORD")
		// Restore dev-mode flag (TestMain default) instead of unsetting it.
		os.Setenv("ENABLE_FAKE_PAYMENT", "true")
	}()

	// Production mode: weak defaults must fail fast.
	os.Setenv("ENABLE_FAKE_PAYMENT", "false")
	cfg, err := Load()
	if err == nil {
		t.Fatal("expected weak-secret rejection in production mode, got nil error")
	}
	if !strings.Contains(err.Error(), "weak default secret") {
		t.Fatalf("expected weak-secret error, got: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config on validation failure")
	}

	// Development mode: same secrets are accepted.
	os.Setenv("ENABLE_FAKE_PAYMENT", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("expected weak secrets to pass in dev mode, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config in dev mode")
	}
}

// TestConfig_LogDefaults verifies LOG_FORMAT/LOG_LEVEL default to json/info
// when unset and are read from the environment when provided.
func TestConfig_LogDefaults(t *testing.T) {
	os.Unsetenv("LOG_FORMAT")
	os.Unsetenv("LOG_LEVEL")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "sk-test")
	os.Setenv("JWT_SECRET", "test-jwt-secret-32-bytes-abcdefghijklmnop")
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.LogFormat != "json" || cfg.Server.LogLevel != "info" {
		t.Errorf("defaults = %s/%s, want json/info", cfg.Server.LogFormat, cfg.Server.LogLevel)
	}

	os.Setenv("LOG_FORMAT", "text")
	os.Setenv("LOG_LEVEL", "debug")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.LogFormat != "text" || cfg.Server.LogLevel != "debug" {
		t.Errorf("env values = %s/%s, want text/debug", cfg.Server.LogFormat, cfg.Server.LogLevel)
	}
}

// TestConfig_ProductionRequiresSecureCookie verifies the fail-fast guard:
// production mode (ENABLE_FAKE_PAYMENT=false) must refuse to start when
// COOKIE_SECURE is false, because auth cookies would traverse plain HTTP.
func TestConfig_ProductionRequiresSecureCookie(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "sk-strong-litellm-key-0123456789")
	os.Setenv("JWT_SECRET", "a-strong-jwt-secret-that-is-way-over-32-bytes")
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	os.Setenv("ADMIN_EMAIL", "admin@test.com")
	os.Setenv("ADMIN_PASSWORD", "a-strong-admin-password-123")
	os.Setenv("ENABLE_FAKE_PAYMENT", "false")
	os.Setenv("COOKIE_SECURE", "false")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ENCRYPTION_KEY")
		os.Unsetenv("ADMIN_EMAIL")
		os.Unsetenv("ADMIN_PASSWORD")
		os.Unsetenv("COOKIE_SECURE")
		// Restore dev-mode flag (TestMain default).
		os.Setenv("ENABLE_FAKE_PAYMENT", "true")
	}()

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected COOKIE_SECURE=false to be rejected in production, got nil error")
	}
	if !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("expected COOKIE_SECURE error, got: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config on validation failure")
	}
}

// TestConfig_ProductionRequiresStrongAdminPassword verifies the fail-fast
// guard: production mode must reject admin passwords shorter than 12 bytes.
func TestConfig_ProductionRequiresStrongAdminPassword(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "sk-strong-litellm-key-0123456789")
	os.Setenv("JWT_SECRET", "a-strong-jwt-secret-that-is-way-over-32-bytes")
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	os.Setenv("ADMIN_EMAIL", "admin@test.com")
	os.Setenv("ADMIN_PASSWORD", "short")
	os.Setenv("ENABLE_FAKE_PAYMENT", "false")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ENCRYPTION_KEY")
		os.Unsetenv("ADMIN_EMAIL")
		os.Unsetenv("ADMIN_PASSWORD")
		// Restore dev-mode flag (TestMain default).
		os.Setenv("ENABLE_FAKE_PAYMENT", "true")
	}()

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected weak admin password to be rejected in production, got nil error")
	}
	if !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("expected ADMIN_PASSWORD error, got: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config on validation failure")
	}
}

// TestConfig_JWTSecretMinimumLength validates that the JWT.Secret
// is at least 32 bytes (256 bits) in length.
func TestConfig_JWTSecretMinimumLength(t *testing.T) {
	tests := []struct {
		name        string
		secret      string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "secret exactly 32 bytes passes",
			secret:      "abcdefghijklmnopqrstuvwxyz123456", // 32 chars
			expectError: false,
		},
		{
			name:        "secret longer than 32 bytes passes",
			secret:      "abcdefghijklmnopqrstuvwxyz1234567890", // 40 chars
			expectError: false,
		},
		{
			name:        "secret shorter than 32 bytes fails",
			secret:      "short-secret",
			expectError: true,
			errorMsg:    "JWT_SECRET must be at least 32 bytes, got 12",
		},
		{
			name:        "secret of 31 bytes fails",
			secret:      "abcdefghijklmnopqrstuvwxyz12345", // 31 chars
			expectError: true,
			errorMsg:    "JWT_SECRET must be at least 32 bytes, got 31",
		},
		{
			name:        "unicode bytes counted correctly",
			secret:      "密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码", // 32 CJK chars, each 3 bytes in UTF-8
			expectError: false,                              // bytes len > 32, should pass
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
			os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
			os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
			os.Setenv("JWT_SECRET", tt.secret)
			os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456") // exactly 32
			os.Setenv("ADMIN_EMAIL", "admin@test.com")
			os.Setenv("ADMIN_PASSWORD", "password123")

			defer func() {
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("LITELLM_BASE_URL")
				os.Unsetenv("LITELLM_MASTER_KEY")
				os.Unsetenv("JWT_SECRET")
				os.Unsetenv("ENCRYPTION_KEY")
				os.Unsetenv("ADMIN_EMAIL")
				os.Unsetenv("ADMIN_PASSWORD")
			}()

			cfg, err := Load()

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("error message mismatch:\n  got:  %q\n  want: %q", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg == nil {
					t.Fatal("expected non-nil config")
				}
				if cfg.JWT.Secret != tt.secret {
					t.Errorf("JWT.Secret = %q, want %q", cfg.JWT.Secret, tt.secret)
				}
			}
		})
	}
}

// TestConfig_EncryptionKeyExactLength ensures the existing encryption key
// validation still works after the JWT validation is added.
func TestConfig_EncryptionKeyExactLength(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "encryption key exactly 32 bytes passes",
			key:         "abcdefghijklmnopqrstuvwxyz123456", // 32 chars
			expectError: false,
		},
		{
			name:        "encryption key shorter than 32 bytes fails",
			key:         "too-short",
			expectError: true,
			errorMsg:    "ENCRYPTION_KEY must be exactly 32 bytes, got 9",
		},
		{
			name:        "encryption key longer than 32 bytes fails",
			key:         "abcdefghijklmnopqrstuvwxyz1234567890", // 36 chars
			expectError: true,
			errorMsg:    "ENCRYPTION_KEY must be exactly 32 bytes, got 36",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
			os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
			os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
			os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456") // 32 chars
			os.Setenv("ENCRYPTION_KEY", tt.key)
			os.Setenv("ADMIN_EMAIL", "admin@test.com")
			os.Setenv("ADMIN_PASSWORD", "password123")

			defer func() {
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("LITELLM_BASE_URL")
				os.Unsetenv("LITELLM_MASTER_KEY")
				os.Unsetenv("JWT_SECRET")
				os.Unsetenv("ENCRYPTION_KEY")
				os.Unsetenv("ADMIN_EMAIL")
				os.Unsetenv("ADMIN_PASSWORD")
			}()

			_, err := Load()

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("error message mismatch:\n  got:  %q\n  want: %q", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestConfig_RedisURLIsOptional verifies that REDIS_URL is no longer required.
func TestConfig_RedisURLIsOptional(t *testing.T) {
	tests := []struct {
		name        string
		redisURL    string
		expectRedis string
	}{
		{
			name:        "REDIS_URL set loads the value",
			redisURL:    "redis://mycache:6379/0",
			expectRedis: "redis://mycache:6379/0",
		},
		{
			name:        "REDIS_URL not set yields empty string",
			redisURL:    "",
			expectRedis: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
			if tt.redisURL != "" {
				os.Setenv("REDIS_URL", tt.redisURL)
			}
			os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
			os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
			os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
			os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")
			os.Setenv("ADMIN_EMAIL", "admin@test.com")
			os.Setenv("ADMIN_PASSWORD", "password123")

			defer func() {
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("REDIS_URL")
				os.Unsetenv("LITELLM_BASE_URL")
				os.Unsetenv("LITELLM_MASTER_KEY")
				os.Unsetenv("JWT_SECRET")
				os.Unsetenv("ENCRYPTION_KEY")
				os.Unsetenv("ADMIN_EMAIL")
				os.Unsetenv("ADMIN_PASSWORD")
			}()

			cfg, err := Load()

			if err != nil {
				t.Fatalf("unexpected error when REDIS_URL is optional: %v", err)
			}
			if cfg == nil {
				t.Fatal("expected non-nil config")
			}
			if cfg.Redis.URL != tt.expectRedis {
				t.Errorf("Redis.URL = %q, want %q", cfg.Redis.URL, tt.expectRedis)
			}
		})
	}
}

// TestConfig_LiteLLMVarsAreOptional verifies that the API starts without
// LITELLM_BASE_URL / LITELLM_MASTER_KEY (LiteLLM is no longer bundled in
// docker-compose; the vars are only needed for an external proxy).
func TestConfig_LiteLLMVarsAreOptional(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")
	os.Setenv("ADMIN_EMAIL", "admin@test.com")
	os.Setenv("ADMIN_PASSWORD", "password123")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ENCRYPTION_KEY")
		os.Unsetenv("ADMIN_EMAIL")
		os.Unsetenv("ADMIN_PASSWORD")
	}()

	os.Unsetenv("LITELLM_BASE_URL")
	os.Unsetenv("LITELLM_MASTER_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error when LITELLM vars are optional: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.LiteLLM.BaseURL != "" || cfg.LiteLLM.MasterKey != "" {
		t.Errorf("LiteLLM config = %q/%q, want empty/empty", cfg.LiteLLM.BaseURL, cfg.LiteLLM.MasterKey)
	}
}

// TestConfig_CookieConfigDefault tests that the CookieConfig defaults
// are set correctly when no env vars are provided.
func TestConfig_CookieConfigDefault(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
	os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")

	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ENCRYPTION_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.Cookie.Name != "auth_token" {
		t.Errorf("Cookie.Name default = %q, want %q", cfg.Cookie.Name, "auth_token")
	}
	if !cfg.Cookie.Secure {
		t.Error("Cookie.Secure default should be true")
	}
	if cfg.Cookie.SameSite != "Strict" {
		t.Errorf("Cookie.SameSite default = %q, want %q", cfg.Cookie.SameSite, "Strict")
	}
	expectedMaxAge := 24 * 3600
	if cfg.Cookie.MaxAgeSeconds != expectedMaxAge {
		t.Errorf("Cookie.MaxAgeSeconds default = %d, want %d", cfg.Cookie.MaxAgeSeconds, expectedMaxAge)
	}
}

// TestConfig_CookieConfigEnvOverrides tests that CookieConfig fields
// can be overridden via environment variables.
func TestConfig_CookieConfigEnvOverrides(t *testing.T) {
	tests := []struct {
		name           string
		envName        string
		envSecure      string
		envSameSite    string
		expectName     string
		expectSecure   bool
		expectSameSite string
	}{
		{name: "override cookie name", envName: "myapp_token", expectName: "myapp_token", expectSecure: true, expectSameSite: "Strict"},
		{name: "override secure to false", envSecure: "false", expectName: "auth_token", expectSecure: false, expectSameSite: "Strict"},
		{name: "override sameSite to Lax", envSameSite: "Lax", expectName: "auth_token", expectSecure: true, expectSameSite: "Lax"},
		{name: "override sameSite to None", envSameSite: "None", expectName: "auth_token", expectSecure: true, expectSameSite: "None"},
		{name: "all overrides combined", envName: "session", envSecure: "false", envSameSite: "Lax", expectName: "session", expectSecure: false, expectSameSite: "Lax"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
			os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
			os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
			os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
			os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")

			if tt.envName != "" {
				os.Setenv("COOKIE_NAME", tt.envName)
			}
			if tt.envSecure != "" {
				os.Setenv("COOKIE_SECURE", tt.envSecure)
			}
			if tt.envSameSite != "" {
				os.Setenv("COOKIE_SAMESITE", tt.envSameSite)
			}

			defer func() {
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("LITELLM_BASE_URL")
				os.Unsetenv("LITELLM_MASTER_KEY")
				os.Unsetenv("JWT_SECRET")
				os.Unsetenv("ENCRYPTION_KEY")
				os.Unsetenv("COOKIE_NAME")
				os.Unsetenv("COOKIE_SECURE")
				os.Unsetenv("COOKIE_SAMESITE")
			}()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Cookie.Name != tt.expectName {
				t.Errorf("Cookie.Name = %q, want %q", cfg.Cookie.Name, tt.expectName)
			}
			if cfg.Cookie.Secure != tt.expectSecure {
				t.Errorf("Cookie.Secure = %v, want %v", cfg.Cookie.Secure, tt.expectSecure)
			}
			if cfg.Cookie.SameSite != tt.expectSameSite {
				t.Errorf("Cookie.SameSite = %q, want %q", cfg.Cookie.SameSite, tt.expectSameSite)
			}
		})
	}
}

// TestConfig_CookieMaxAgeFromJWTDuration verifies MaxAgeSeconds
// is computed from JWT.ExpiryHours.
func TestConfig_CookieMaxAgeFromJWTDuration(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("LITELLM_BASE_URL", "http://localhost:4000")
	os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
	os.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	os.Setenv("JWT_EXPIRY_HOURS", "48")
	os.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")

	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("LITELLM_BASE_URL")
		os.Unsetenv("LITELLM_MASTER_KEY")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_EXPIRY_HOURS")
		os.Unsetenv("ENCRYPTION_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedMaxAge := 48 * 3600
	if cfg.Cookie.MaxAgeSeconds != expectedMaxAge {
		t.Errorf("Cookie.MaxAgeSeconds = %d, want %d", cfg.Cookie.MaxAgeSeconds, expectedMaxAge)
	}
}
