//go:build ignore

// gc_test_schemas 清理 per-package 测试隔离机制遗留的 schema。
//
// 每个 Go 测试包每次运行都会在 *_test 数据库里新建一个
// "t_<package>_<8位hex>" schema（测试结束后不删除，避免跨包并发互踩），
// 长时间运行会在测试库积累大量空/残留 schema。本脚本负责回收。
//
// 安全护栏（防止重蹈 DROP SCHEMA public 事故）：
//  1. TEST_DATABASE_URL 必须指向库名以 _test 结尾的数据库；
//  2. 只匹配 harness 生成的精确模式 ^t_[a-z0-9_]+_[0-9a-f]{8}$；
//  3. 默认 dry-run 只列不删，必须显式 -apply 才执行 DROP SCHEMA ... CASCADE。
//
// 用法：
//
//	TEST_DATABASE_URL=... go run scripts/gc_test_schemas.go          # 列出
//	TEST_DATABASE_URL=... go run scripts/gc_test_schemas.go -apply    # 回收
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

const harnessSchemaPattern = `^t_[a-z0-9_]+_[0-9a-f]{8}$`

func main() {
	apply := flag.Bool("apply", false, "actually DROP matching schemas (default: dry-run)")
	flag.Parse()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "TEST_DATABASE_URL is required")
		os.Exit(1)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse TEST_DATABASE_URL: %v\n", err)
		os.Exit(1)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		fmt.Fprintf(os.Stderr, "refusing to run GC against non-test database %q\n", cfg.ConnConfig.Database)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, "SELECT nspname FROM pg_namespace WHERE nspname ~ $1 ORDER BY nspname", harnessSchemaPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query schemas: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			fmt.Fprintf(os.Stderr, "scan: %v\n", err)
			os.Exit(1)
		}
		schemas = append(schemas, name)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "iterate: %v\n", err)
		os.Exit(1)
	}

	if len(schemas) == 0 {
		fmt.Println("no harness schemas found")
		return
	}
	for _, s := range schemas {
		fmt.Println(s)
	}
	if !*apply {
		fmt.Printf("%d schema(s) would be dropped; rerun with -apply to delete them\n", len(schemas))
		return
	}

	nameRE := regexp.MustCompile(harnessSchemaPattern)
	for _, s := range schemas {
		// Second line of defense: the SQL above already constrains the set,
		// but never DROP a name that does not match the exact harness pattern.
		if !nameRE.MatchString(s) {
			fmt.Fprintf(os.Stderr, "skipping %q: does not match harness pattern\n", s)
			continue
		}
		if _, err := pool.Exec(ctx, "DROP SCHEMA "+s+" CASCADE"); err != nil {
			fmt.Fprintf(os.Stderr, "drop %s: %v\n", s, err)
			continue
		}
		fmt.Printf("dropped %s\n", s)
	}
}
