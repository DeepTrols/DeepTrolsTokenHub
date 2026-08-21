//go:build ignore

// probe_pricing 用真实开发库跑一遍 pricer，验证 B1 成本/售价双通道与峰谷定价。
// 用法: go run ./scripts/probe_pricing.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/service/billing"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	models := model.NewPostgresRepository(pool)
	pricer := billing.NewPricer(models)

	for _, code := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		m, err := models.FindByCode(ctx, code)
		if err != nil {
			fmt.Printf("%s: model not found: %v\n", code, err)
			continue
		}
		usage := &usageparser.NormalizedUsage{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 2000}
		shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
		for _, now := range []time.Time{
			time.Date(2026, 8, 21, 10, 0, 0, 0, shanghai), // 高峰
			time.Date(2026, 8, 21, 20, 0, 0, 0, shanghai), // 非高峰
		} {
			res, err := pricer.CalculateAt(ctx, m.ID, nil, usage, now)
			if err != nil {
				fmt.Printf("%s %s: error: %v\n", code, now.Format("15:04"), err)
				continue
			}
			fmt.Printf("%s %s period=%s sell=%s cost=%s missing=%v\n",
				code, now.Format("15:04"), res.Period, res.ListCost, res.UpstreamCost, res.MissingPricing)
		}
	}
}
