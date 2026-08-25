//go:build ignore
// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}

	dsKey := os.Getenv("DEEPSEEK_API_KEY")
	qwKey := os.Getenv("QWEN_API_KEY")
	if dsKey == "" || qwKey == "" {
		fmt.Fprintln(os.Stderr, "DEEPSEEK_API_KEY and QWEN_API_KEY must be set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	type modelDef struct {
		ID, Code, Provider, Category, DisplayName, Description string
		ContextWindow, MaxOutputTokens                         int
	}

	models := []modelDef{
		{"a0000001-0001-0001-0001-000000000001", "deepseek-v4-flash", "deepseek", "chat", "DeepSeek V4 Flash", "DeepSeek 快速模型", 131072, 32768},
		{"a0000001-0001-0001-0001-000000000002", "deepseek-v4-pro", "deepseek", "chat", "DeepSeek V4 Pro", "DeepSeek 旗舰推理模型", 131072, 65536},
		{"a0000001-0001-0001-0001-000000000007", "deepseek-v4-flash-vision-exp", "deepseek", "chat", "DeepSeek V4 Flash Vision", "DeepSeek 视觉增强模型", 131072, 32768},
		{"a0000001-0001-0001-0001-000000000003", "qwen3.5-plus", "qwen", "chat", "Qwen 3.5 Plus", "通义千问 3.5 Plus", 131072, 16384},
		{"a0000001-0001-0001-0001-000000000004", "qwen3.5-flash", "qwen", "chat", "Qwen 3.5 Flash", "通义千问 3.5 Flash 轻量", 131072, 8192},
		{"a0000001-0001-0001-0001-000000000005", "qwen3.7-plus", "qwen", "chat", "Qwen 3.7 Plus", "通义千问 3.7 Plus", 131072, 16384},
		{"a0000001-0001-0001-0001-000000000006", "qwen3.7-max", "qwen", "chat", "Qwen 3.7 Max", "通义千问 3.7 Max 旗舰", 131072, 65536},
	}

	for _, m := range models {
		sql := `INSERT INTO models (id,code,provider,category,display_name,description,context_window,max_output_tokens,status,release_stage,created_at,updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active','GA',NOW(),NOW())
				ON CONFLICT (code) DO UPDATE SET display_name=EXCLUDED.display_name,updated_at=NOW()`
		_, err := pool.Exec(ctx, sql, m.ID, m.Code, m.Provider, m.Category, m.DisplayName, m.Description, m.ContextWindow, m.MaxOutputTokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "model %s: %v\n", m.Code, err)
			os.Exit(1)
		}
		fmt.Printf("OK model: %s\n", m.Code)
	}

	type pricingDef struct{ ModelID, Dim, Price string }
	pricings := []pricingDef{
		{"a0000001-0001-0001-0001-000000000001", "input", "0.002"},
		{"a0000001-0001-0001-0001-000000000001", "output", "0.006"},
		{"a0000001-0001-0001-0001-000000000002", "input", "0.004"},
		{"a0000001-0001-0001-0001-000000000002", "output", "0.012"},
		{"a0000001-0001-0001-0001-000000000007", "input", "0.002"},
		{"a0000001-0001-0001-0001-000000000007", "output", "0.006"},
		{"a0000001-0001-0001-0001-000000000003", "input", "0.001"},
		{"a0000001-0001-0001-0001-000000000003", "output", "0.003"},
		{"a0000001-0001-0001-0001-000000000004", "input", "0.0005"},
		{"a0000001-0001-0001-0001-000000000004", "output", "0.0015"},
		{"a0000001-0001-0001-0001-000000000005", "input", "0.001"},
		{"a0000001-0001-0001-0001-000000000005", "output", "0.003"},
		{"a0000001-0001-0001-0001-000000000006", "input", "0.004"},
		{"a0000001-0001-0001-0001-000000000006", "output", "0.012"},
	}
	for _, p := range pricings {
		sql := `INSERT INTO model_pricing (id,model_id,tenant_id,request_type,pricing_dimension,unit_name,unit_price,upstream_cost,currency,created_at,updated_at)
				VALUES (uuid_generate_v4(),$1::uuid,NULL,'chat',$2,'1K tokens',$3,$3,'CNY',NOW(),NOW())`
		_, err := pool.Exec(ctx, sql, p.ModelID, p.Dim, p.Price)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pricing %s/%s: %v\n", p.ModelID, p.Dim, err)
		}
	}
	fmt.Println("OK pricing: done")

	type channelDef struct{ ID, Name, ModelID string }
	channels := []channelDef{
		{"b0000001-0001-0001-0001-000000000001", "deepseek-flash", "a0000001-0001-0001-0001-000000000001"},
		{"b0000001-0001-0001-0001-000000000002", "deepseek-pro", "a0000001-0001-0001-0001-000000000002"},
		{"b0000001-0001-0001-0001-000000000007", "deepseek-vision", "a0000001-0001-0001-0001-000000000007"},
		{"b0000001-0001-0001-0001-000000000003", "qwen35-plus", "a0000001-0001-0001-0001-000000000003"},
		{"b0000001-0001-0001-0001-000000000004", "qwen35-flash", "a0000001-0001-0001-0001-000000000004"},
		{"b0000001-0001-0001-0001-000000000005", "qwen37-plus", "a0000001-0001-0001-0001-000000000005"},
		{"b0000001-0001-0001-0001-000000000006", "qwen37-max", "a0000001-0001-0001-0001-000000000006"},
	}
	for _, ch := range channels {
		sql := `INSERT INTO channels (id,name,model_id,pool_type,health_score,health_status,status,weight,max_concurrency,created_at,updated_at)
				VALUES ($1,$2,$3,'shared',100,'healthy','active',100,10,NOW(),NOW()) ON CONFLICT DO NOTHING`
		_, err := pool.Exec(ctx, sql, ch.ID, ch.Name, ch.ModelID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "channel %s: %v\n", ch.Name, err)
			os.Exit(1)
		}
		fmt.Printf("OK channel: %s\n", ch.Name)
	}

	type instDef struct{ ID, ChannelID, BaseURL, Route, Key, ProviderType, DisplayName string }
	instances := []instDef{
		{"c0000001-0001-0001-0001-000000000001", "b0000001-0001-0001-0001-000000000001", "https://api.deepseek.com", "deepseek-v4-flash", dsKey, "deepseek", "DeepSeek"},
		{"c0000001-0001-0001-0001-000000000002", "b0000001-0001-0001-0001-000000000002", "https://api.deepseek.com", "deepseek-v4-pro", dsKey, "deepseek", "DeepSeek"},
		{"c0000001-0001-0001-0001-000000000007", "b0000001-0001-0001-0001-000000000007", "https://api.deepseek.com", "deepseek-v4-flash-vision-exp", dsKey, "deepseek", "DeepSeek"},
		{"c0000001-0001-0001-0001-000000000003", "b0000001-0001-0001-0001-000000000003", "https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "qwen3.5-plus", qwKey, "qwen", "Qwen 通义千问"},
		{"c0000001-0001-0001-0001-000000000004", "b0000001-0001-0001-0001-000000000004", "https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "qwen3.5-flash", qwKey, "qwen", "Qwen 通义千问"},
		{"c0000001-0001-0001-0001-000000000005", "b0000001-0001-0001-0001-000000000005", "https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "qwen3.7-plus", qwKey, "qwen", "Qwen 通义千问"},
		{"c0000001-0001-0001-0001-000000000006", "b0000001-0001-0001-0001-000000000006", "https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "qwen3.7-max", qwKey, "qwen", "Qwen 通义千问"},
	}
	for _, inst := range instances {
		cfg, _ := json.Marshal(map[string]string{
			"api_key":      inst.Key,
			"provider":     inst.ProviderType,
			"display_name": inst.DisplayName,
		})
		sql := `INSERT INTO channel_instances (id,channel_id,instance_type,base_url,provider_route,current_load,max_load,config,status,created_at,updated_at)
				VALUES ($1,$2,'serverless',$3,$4,0,10,$5,'active',NOW(),NOW()) ON CONFLICT DO NOTHING`
		_, err := pool.Exec(ctx, sql, inst.ID, inst.ChannelID, inst.BaseURL, inst.Route, string(cfg))
		if err != nil {
			fmt.Fprintf(os.Stderr, "instance %s: %v\n", inst.ID, err)
			os.Exit(1)
		}
		fmt.Printf("OK instance: %s -> %s\n", inst.Route, inst.BaseURL)
	}

	var count int
	pool.QueryRow(ctx, "SELECT count(*) FROM models WHERE status='active'").Scan(&count)
	fmt.Printf("\nSeed complete. %d active models.\n", count)
}
