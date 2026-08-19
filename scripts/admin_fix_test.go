//go:build ignore

// One-off dev script: fixes the legacy admin row (email/password) in the local
// deeptrols database. Build-ignored so `go test ./...` can never run the
// hardcoded UPDATE against a live database; run manually when needed with
// `go test ./scripts/ -run TestFixAdmin` after removing the build tag.
package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestFixAdmin(t *testing.T) {
	u := "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte("deeptrols@2026"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}

	r, err := pool.Exec(ctx,
		"UPDATE users SET email=$1, password_hash=$2 WHERE email='admin'",
		"deeptrols@admin.com", string(hash),
	)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Updated %d rows\n", r.RowsAffected())
	if r.RowsAffected() == 0 {
		t.Skip("no admin user to update")
	}
}
