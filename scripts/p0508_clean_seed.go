//go:build ignore

// Command p0508_clean_seed seeds a disposable clean-deployment database for
// the TH-P05-08 verification run: one verification user + API key + funded
// wallet (via the production ledgered provision path) and one fake
// model/pricing/channel/instance chain pointing at the local echo upstream.
//
// Every row is tagged "p0508-deployment-verification" (audit requirement:
// admin writes during deployment verification must be tagged).
//
// Usage (all via env so secrets never sit in argv):
//
//	P0508_DATABASE_URL=postgresql://... \
//	P0508_ENCRYPTION_KEY=<32 bytes, same as the API> \
//	P0508_PLAINTEXT_KEY=sk-p0508-... \
//	P0508_FAKE_BASE_URL=http://127.0.0.1:8090 \
//	go run scripts/p0508_clean_seed.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/deeptrols/api/internal/handler/console"
	"github.com/deeptrols/api/internal/pkg/keyhash"
	"github.com/deeptrols/api/internal/repository/wallet"
)

const tag = "p0508-deployment-verification"

func envRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required env %s is not set\n", key)
		os.Exit(2)
	}
	return v
}

func mustExec(ctx context.Context, pool *pgxpool.Pool, label, sql string, args ...any) {
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
		os.Exit(1)
	}
	fmt.Printf("OK: %s\n", label)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbURL := envRequired("P0508_DATABASE_URL")
	encKey := envRequired("P0508_ENCRYPTION_KEY")
	plainKey := envRequired("P0508_PLAINTEXT_KEY")
	baseURL := os.Getenv("P0508_FAKE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8090"
	}
	modelCode := os.Getenv("P0508_MODEL_CODE")
	if modelCode == "" {
		modelCode = "fake-chat-1"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 1. Verification user (login disabled: unusable password hash).
	userID := uuid.New()
	mustExec(ctx, pool, "verification user",
		`INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		 VALUES ($1, $2, 'disabled-by-deployment-verification', $3, 'user', 'active', NOW(), NOW())`,
		userID, tag+"@deeptrols.local", "P0508 Deployment Verification")

	// 2. API key (HMAC-SHA256 of the plaintext key under ENCRYPTION_KEY,
	// exactly the shape GatewayAuth verifies).
	mustExec(ctx, pool, "verification api key",
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active', NOW(), NOW())`,
		uuid.New(), userID, plainKey[:10], keyhash.Hash(plainKey, encKey),
		plainKey[:6]+"****", tag)

	// 3. Wallet funded through the production ledgered provision path.
	wallets := wallet.NewPostgresRepository(pool)
	w, err := console.ProvisionUserWallet(ctx, wallets, userID, decimal.NewFromInt(5))
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision wallet: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: wallet %s funded to %s via ledgered TopUp\n", w.ID, w.Balance)

	// 4. Model.
	modelID := uuid.New()
	mustExec(ctx, pool, "verification model",
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, $2, 'fake', 'chat', $3, 'active', 'GA', NOW(), NOW())`,
		modelID, modelCode, "P0508 Fake Chat ("+tag+")")

	// 5. Pricing: 1.0 / 2.0 CNY per 1M tokens (input/output). Echo upstream
	// reports 1+1 tokens, so the exact settled cost must be 0.000003.
	for _, row := range []struct{ dim, price, cost string }{
		{"input", "1.0", "0.5"},
		{"output", "2.0", "1.0"},
	} {
		mustExec(ctx, pool, "pricing "+row.dim,
			`INSERT INTO model_pricing (id, model_id, tenant_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
			 VALUES ($1, $2, NULL, 'chat', $3, '1M tokens', $4, 'CNY', $5, TRUE, NOW(), NOW())`,
			uuid.New(), modelID, row.dim, row.price, row.cost)
	}

	// 6. Channel + instance pointing at the local echo upstream.
	channelID := uuid.New()
	mustExec(ctx, pool, "verification channel",
		`INSERT INTO channels (id, name, model_id, tenant_id, pool_type, health_score, health_status, status, weight, max_concurrency, created_at, updated_at)
		 VALUES ($1, $2, $3, NULL, 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW())`,
		channelID, tag+"-channel", modelID)

	instCfg, _ := json.Marshal(map[string]string{
		"api_key":  "fake-upstream-key-" + tag,
		"provider": "fake",
	})
	mustExec(ctx, pool, "verification channel instance",
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, current_load, max_load, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'serverless', $3, $4, 0, 10, $5, 'active', NOW(), NOW())`,
		uuid.New(), channelID, baseURL+"/v1", modelCode, string(instCfg))

	fmt.Printf("\nSeeded %s:\n  user=%s\n  model=%s (id=%s)\n  gateway base=%s\n  expected settled cost=0.000003 CNY\n",
		tag, userID, modelCode, modelID, baseURL)
}
