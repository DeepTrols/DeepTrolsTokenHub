//go:build ignore
// +build ignore

package main

import (
	"context"
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, "SELECT id, provider FROM models WHERE status = 'active'")
	if err != nil {
		fmt.Fprintf(os.Stderr, "query: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	type modelRow struct{ ID, Provider string }
	var models []modelRow
	for rows.Next() {
		var m modelRow
		if err := rows.Scan(&m.ID, &m.Provider); err != nil {
			fmt.Fprintf(os.Stderr, "scan: %v\n", err)
			continue
		}
		models = append(models, m)
	}

	for _, m := range models {
		for _, dim := range []string{"input", "output"} {
			price := "0.001"
			if m.Provider == "qwen" {
				if dim == "output" {
					price = "0.003"
				}
			} else {
				if dim == "input" {
					price = "0.002"
				} else {
					price = "0.006"
				}
			}
			sql := `INSERT INTO model_pricing (id,model_id,tenant_id,request_type,pricing_dimension,unit_name,unit_price,upstream_cost,currency,created_at,updated_at)
				VALUES (uuid_generate_v4(),$1,NULL,'chat',$2,'1K tokens',$3,$3,'CNY',NOW(),NOW())
				ON CONFLICT DO NOTHING`
			_, err := pool.Exec(ctx, sql, m.ID, dim, price)
			if err != nil {
				fmt.Fprintf(os.Stderr, "pricing %s/%s: %v\n", m.ID, dim, err)
			} else {
				fmt.Printf("OK pricing: %s %s = %s\n", m.Provider, dim, price)
			}
		}
	}

	var count int
	pool.QueryRow(ctx, "SELECT count(*) FROM model_pricing").Scan(&count)
	fmt.Printf("\nDone. %d pricing rows.\n", count)
}
