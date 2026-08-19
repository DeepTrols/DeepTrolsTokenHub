// Command cleanup_residue_models removes the bytedance model residue created
// by early provider auto-discovery during un-isolated testing (130 models,
// never used). Dry-run by default; pass -apply to delete after writing a
// restore script to backups/.
//
// It deliberately targets DATABASE_URL (the dev database), refuses *_test
// databases, and only touches models whose provider is "bytedance" and that
// have zero usage_logs (belt-and-suspenders: real usage is never deleted).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	apply := flag.Bool("apply", false, "write backup and delete; default is dry-run")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse DATABASE_URL: %v\n", err)
		os.Exit(1)
	}
	if strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		fmt.Fprintln(os.Stderr, "refusing to run against a *_test database; this tool targets the dev database")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Target: bytedance models with zero usage. usage_logs stores the model
	// code (no FK), so the guard is by code.
	rows, err := pool.Query(ctx, `
		SELECT m.id, m.code
		FROM models m
		WHERE m.provider = 'bytedance'
		  AND NOT EXISTS (
		      SELECT 1 FROM usage_logs ul WHERE ul.public_model_code = m.code
		  )
		ORDER BY m.code`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan models: %v\n", err)
		os.Exit(1)
	}
	type modelRef struct{ id, code string }
	var models []modelRef
	for rows.Next() {
		var m modelRef
		if err := rows.Scan(&m.id, &m.code); err != nil {
			fmt.Fprintf(os.Stderr, "scan model row: %v\n", err)
			os.Exit(1)
		}
		models = append(models, m)
	}
	rows.Close()

	fmt.Printf("target models (provider=bytedance, zero usage): %d\n", len(models))
	if len(models) == 0 {
		fmt.Println("nothing to do")
		return
	}

	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, "'"+m.id+"'")
	}
	idList := strings.Join(ids, ",")
	codeList := "'" + strings.Join(func() []string {
		out := make([]string, 0, len(models))
		for _, m := range models {
			out = append(out, strings.ReplaceAll(m.code, "'", "''"))
		}
		return out
	}(), "','") + "'"

	counts := map[string]int{}
	for _, q := range []struct {
		name string
		sql  string
	}{
		{"route_policies", "SELECT COUNT(*) FROM route_policies WHERE model_id IN (" + idList + ")"},
		{"channel_instances", "SELECT COUNT(*) FROM channel_instances WHERE channel_id IN (SELECT id FROM channels WHERE model_id IN (" + idList + "))"},
		{"channels", "SELECT COUNT(*) FROM channels WHERE model_id IN (" + idList + ")"},
		{"tenant_models", "SELECT COUNT(*) FROM tenant_models WHERE model_id IN (" + idList + ")"},
		{"model_pricing", "SELECT COUNT(*) FROM model_pricing WHERE model_id IN (" + idList + ")"},
		{"quota_pools", "SELECT COUNT(*) FROM quota_pools WHERE model_id IN (" + idList + ")"},
		{"usage_logs(match by code, must be 0)", "SELECT COUNT(*) FROM usage_logs WHERE public_model_code IN (" + codeList + ")"},
	} {
		var n int
		if err := pool.QueryRow(ctx, q.sql).Scan(&n); err != nil {
			fmt.Fprintf(os.Stderr, "count %s: %v\n", q.name, err)
			os.Exit(1)
		}
		counts[q.name] = n
		fmt.Printf("  %-38s %d\n", q.name, n)
	}

	if counts["usage_logs(match by code, must be 0)"] != 0 {
		fmt.Fprintln(os.Stderr, "aborting: usage exists for target model codes")
		os.Exit(1)
	}
	if !*apply {
		fmt.Println("dry-run: pass -apply to delete (a restore script is written to backups/ first)")
		return
	}

	backupDir := "backups/2026-08-19"
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir backup dir: %v\n", err)
		os.Exit(1)
	}
	backupFile := filepath.Join(backupDir, "residue_models_"+time.Now().Format("20060102_150405")+".sql")
	f, err := os.Create(backupFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create backup: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Write restore data for the FK-referencing tables + models. Each table is
	// dumped as a COPY ... FROM stdin block in PostgreSQL text format, so the
	// file can be restored with: psql "$DATABASE_URL" < file.
	for _, t := range []struct {
		name string
		sql  string
	}{
		{"model_pricing", "SELECT * FROM model_pricing WHERE model_id IN (" + idList + ")"},
		{"tenant_models", "SELECT * FROM tenant_models WHERE model_id IN (" + idList + ")"},
		{"route_policies", "SELECT * FROM route_policies WHERE model_id IN (" + idList + ")"},
		{"channels", "SELECT * FROM channels WHERE model_id IN (" + idList + ")"},
		{"channel_instances", "SELECT * FROM channel_instances WHERE channel_id IN (SELECT id FROM channels WHERE model_id IN (" + idList + "))"},
		{"models", "SELECT * FROM models WHERE id IN (" + idList + ")"},
	} {
		dumpTable(ctx, pool, f, t.name, t.sql)
	}
	fmt.Fprintf(f, "-- Restore with: psql \"$DATABASE_URL\" < %s\n", backupFile)
	fmt.Fprintf(os.Stdout, "backup written: %s\n", backupFile)

	// Delete in FK-safe order (all FKs are RESTRICT, no CASCADE).
	tx, err := pool.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin tx: %v\n", err)
		os.Exit(1)
	}
	defer tx.Rollback(ctx)
	for _, q := range []struct {
		name string
		sql  string
	}{
		{"route_policies", "DELETE FROM route_policies WHERE model_id IN (" + idList + ")"},
		{"channel_instances", "DELETE FROM channel_instances WHERE channel_id IN (SELECT id FROM channels WHERE model_id IN (" + idList + "))"},
		{"channels", "DELETE FROM channels WHERE model_id IN (" + idList + ")"},
		{"tenant_models", "DELETE FROM tenant_models WHERE model_id IN (" + idList + ")"},
		{"model_pricing", "DELETE FROM model_pricing WHERE model_id IN (" + idList + ")"},
		{"models", "DELETE FROM models WHERE id IN (" + idList + ")"},
	} {
		tag, err := tx.Exec(ctx, q.sql)
		if err != nil {
			fmt.Fprintf(os.Stderr, "delete %s: %v\n", q.name, err)
			os.Exit(1)
		}
		fmt.Printf("deleted %-20s %d\n", q.name, tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "commit: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("done")
}

// dumpTable writes a COPY ... FROM stdin block (PostgreSQL text format) for
// every row of a query to w, so a mistaken cleanup can be rolled back with
// psql. pgx's RawValues are binary-encoded for most types, so they must not be
// spliced into INSERT statements; COPY TO STDOUT text format keeps the backup
// human-readable and directly restorable.
func dumpTable(ctx context.Context, pool *pgxpool.Pool, f *os.File, table, sql string) {
	// Resolve the column list from a zero-row scan of the same query.
	colsRows, err := pool.Query(ctx, sql+" LIMIT 0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup %s columns: %v\n", table, err)
		os.Exit(1)
	}
	fields := colsRows.FieldDescriptions()
	colsRows.Close()
	names := make([]string, len(fields))
	for i, fd := range fields {
		names[i] = `"` + fd.Name + `"`
	}
	fmt.Fprintf(f, "COPY %s (%s) FROM stdin;\n", table, strings.Join(names, ", "))

	conn, err := pool.Acquire(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup %s acquire: %v\n", table, err)
		os.Exit(1)
	}
	defer conn.Release()
	if _, err := conn.Conn().PgConn().CopyTo(ctx, f, "COPY ("+sql+") TO STDOUT"); err != nil {
		fmt.Fprintf(os.Stderr, "backup %s copy: %v\n", table, err)
		os.Exit(1)
	}
	fmt.Fprintln(f, `\.`)
}
