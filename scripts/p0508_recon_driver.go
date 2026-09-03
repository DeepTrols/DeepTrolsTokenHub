//go:build ignore

// Command p0508_recon_driver runs ONE reconciliation cycle on a disposable
// database using the exact production reconciler (internal/worker/
// reconciliation.Reconciler.Run), proving AC-04 of TH-P05-08 without waiting
// for the worker's 1-hour ticker. The worker-side lease that guards this code
// path in production is verified separately via the worker:lease:* Redis keys.
//
// Usage:
//
//	P0508_DATABASE_URL=postgresql://... go run scripts/p0508_recon_driver.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deeptrols/api/internal/worker/reconciliation"
)

func main() {
	dbURL := os.Getenv("P0508_DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "required env P0508_DATABASE_URL is not set")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := reconciliation.New(pool).Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reconciliation run failed: %v\n", err)
		os.Exit(1)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reconciliation_runs`).Scan(&count); err != nil {
		fmt.Fprintf(os.Stderr, "count runs: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("reconciliation run completed; reconciliation_runs rows = %d\n", count)
}
