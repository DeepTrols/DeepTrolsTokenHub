//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		u = "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, u)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	pool.Exec(ctx, "TRUNCATE channel_instances, channels, model_pricing, models CASCADE")
	var mc, cc, ic int
	pool.QueryRow(ctx, "SELECT count(*) FROM models").Scan(&mc)
	pool.QueryRow(ctx, "SELECT count(*) FROM channels").Scan(&cc)
	pool.QueryRow(ctx, "SELECT count(*) FROM channel_instances").Scan(&ic)
	fmt.Printf("Cleaned. models=%d channels=%d instances=%d\n", mc, cc, ic)
}
