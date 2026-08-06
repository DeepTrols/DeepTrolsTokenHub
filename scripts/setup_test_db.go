//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pool, err := pgxpool.New(context.Background(), "postgresql://deeptrols:deeptrols_dev@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	_, err = pool.Exec(context.Background(), "CREATE DATABASE deeptrols_test")
	if err != nil {
		fmt.Printf("Note: %v (may already exist)\n", err)
	} else {
		fmt.Println("Created database: deeptrols_test")
	}

	fmt.Println("Done. Run: migrate -path migrations -database \"postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols_test?sslmode=disable\" up")
}
