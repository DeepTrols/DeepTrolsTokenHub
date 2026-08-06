//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols_test?sslmode=disable")
	if err != nil {
		fmt.Printf("Connect error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	migrationsDir := `G:\workspace\demo\deeptrols-api\migrations`
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		fmt.Printf("ReadDir error: %v\n", err)
		os.Exit(1)
	}

	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, f := range upFiles {
		path := filepath.Join(migrationsDir, f)
		sql, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("ReadFile %s: %v\n", f, err)
			os.Exit(1)
		}
		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			fmt.Printf("Migrate %s FAILED: %v\n", f, err)
			os.Exit(1)
		}
		fmt.Printf("OK: %s\n", f)
	}
	fmt.Println("All migrations applied to deeptrols_test!")
}
