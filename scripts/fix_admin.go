//go:build ignore

package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
)

func main() {
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		u = "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"
	}
	ctx := context.Background()
	p, _ := pgxpool.New(ctx, u)
	defer p.Close()
	p.Exec(ctx, "DELETE FROM users WHERE email IN ('admin','deeptrols@admin.com')")
	var c int
	p.QueryRow(ctx, "SELECT count(*) FROM users WHERE email='deeptrols@admin.com'").Scan(&c)
	fmt.Printf("Admin DB records: %d\n", c)
}
