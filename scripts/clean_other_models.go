//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/deeptrols/api/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DB.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	ctx := context.Background()

	// FK chain: usage_logs → channel_instances → channels → model_pricing → models
	otherModels := `SELECT id FROM models WHERE provider = 'other'`
	otherChannels := `SELECT id FROM channels WHERE model_id IN (` + otherModels + `)`
	otherInstances := `SELECT id FROM channel_instances WHERE channel_id IN (` + otherChannels + `)`

	for _, sql := range []string{
		`UPDATE usage_logs SET instance_id = NULL, channel_id = NULL WHERE instance_id IN (` + otherInstances + `) OR channel_id IN (` + otherChannels + `)`,
		`DELETE FROM channel_instances WHERE id IN (` + otherInstances + `)`,
		`DELETE FROM channels WHERE id IN (` + otherChannels + `)`,
		`DELETE FROM model_pricing WHERE model_id IN (` + otherModels + `)`,
		`DELETE FROM models WHERE provider = 'other'`,
	} {
		tag, err := pool.Exec(ctx, sql)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  %d rows deleted\n", tag.RowsAffected())
	}

	rows, _ := pool.Query(ctx, `SELECT provider, count(*) FROM models GROUP BY provider ORDER BY count DESC`)
	defer rows.Close()
	fmt.Println("\nRemaining:")
	for rows.Next() {
		var pv string
		var n int
		rows.Scan(&pv, &n)
		fmt.Printf("  %-15s %d\n", pv, n)
	}
}
