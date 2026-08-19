package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/deeptrols/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schemaNamePattern is the exact naming scheme of harness-created schemas:
// t_<package_key>_<8 hex chars>. GC and the DROP guards rely on this pattern.
var schemaNamePattern = regexp.MustCompile(`^t_[a-z0-9_]+_[0-9a-f]{8}$`)

// pkgKeyPattern bounds the derived package key before it is embedded in a
// schema identifier.
var pkgKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,50}$`)

var (
	packageStatesMu sync.Mutex
	packageStates   = map[string]*packageState{}
)

// packageState is the per-process provisioning state for one test package.
// A package provisions exactly one schema (once) and all of its tests share
// it; tests within a package already truncate between cases.
type packageState struct {
	once   sync.Once
	schema string
	err    error
}

// SchemaDSN returns a TEST_DATABASE_URL-derived DSN that routes every query of
// the calling test package into its own private schema. The schema is created
// and fully migrated on first use. It skips the test when TEST_DATABASE_URL is
// unset and fails when the DSN does not point at a *_test database.
func SchemaDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
		return ""
	}
	assertTestDatabase(t, dsn)

	st := stateForPackage(t)
	st.ensure(t, dsn)

	out, err := schemaDSN(dsn, st.schema)
	if err != nil {
		t.Fatalf("testutil: build schema DSN: %v", err)
	}
	return out
}

func (s *packageState) ensure(t *testing.T, dsn string) {
	t.Helper()
	s.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.err = provisionSchema(ctx, dsn, s.schema)
	})
	if s.err != nil {
		t.Fatalf("testutil: provision schema %s: %v", s.schema, s.err)
	}
}

// stateForPackage returns the provisioning state for the calling package.
func stateForPackage(t *testing.T) *packageState {
	t.Helper()

	key := callerPackageKey()
	if key == "unknown" || key == "" {
		t.Fatal("testutil: could not determine calling test package; refusing to provision a schema")
	}

	packageStatesMu.Lock()
	defer packageStatesMu.Unlock()
	if st, ok := packageStates[key]; ok {
		return st
	}
	schema, err := schemaNameForKey(key)
	if err != nil {
		t.Fatalf("testutil: %v", err)
	}
	st := &packageState{schema: schema}
	packageStates[key] = st
	return st
}

// callerPackageKey walks the runtime stack to the first frame outside this
// package's helper files and derives a stable package key from its location.
func callerPackageKey() string {
	for depth := 1; depth < 64; depth++ {
		_, file, _, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		if isTestutilHelperFrame(file) {
			continue
		}
		if key := pkgKeyFromFile(file); key != "" {
			return key
		}
	}
	return "unknown"
}

// isTestutilHelperFrame reports whether the file is one of this package's
// non-test helper files. Test files inside this package (schema_test.go) are
// intentionally NOT treated as helpers so the package's own tests get their
// own isolated schema key.
func isTestutilHelperFrame(file string) bool {
	norm := filepath.ToSlash(file)
	if !strings.Contains(norm, "/internal/repository/testutil/") {
		return false
	}
	base := filepath.Base(file)
	return base == "schema.go" || base == "db.go"
}

// pkgKeyFromFile derives a stable package key from a source file path, e.g.
// .../internal/handler/console/ledger_test.go -> internal_handler_console.
// Files outside internal/ or cmd/ yield "".
func pkgKeyFromFile(file string) string {
	norm := filepath.ToSlash(file)
	for _, marker := range []string{"/internal/", "/cmd/"} {
		if idx := strings.Index(norm, marker); idx >= 0 {
			dir := filepath.ToSlash(filepath.Dir(norm[idx+1:]))
			return sanitizeKey(dir)
		}
	}
	return ""
}

func sanitizeKey(dir string) string {
	dir = strings.Trim(dir, "/")
	return strings.ReplaceAll(dir, "/", "_")
}

// schemaNameForKey builds a unique, valid schema identifier for a package key.
func schemaNameForKey(key string) (string, error) {
	if !pkgKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid package key %q", key)
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate schema suffix: %w", err)
	}
	name := "t_" + key + "_" + hex.EncodeToString(b[:])
	if len(name) > 63 {
		return "", fmt.Errorf("schema name %q exceeds 63 bytes", name)
	}
	return name, nil
}

// validSchemaName reports whether name is one of the harness-created schemas.
func validSchemaName(name string) bool {
	return schemaNamePattern.MatchString(name)
}

// schemaDSN appends an options runtime parameter that pins search_path to the
// private schema (public remains reachable so database-level extensions and
// objects stay visible). pgx does not honor search_path as a raw startup
// parameter, but PostgreSQL processes options=-c ... as command-line settings.
func schemaDSN(dsn, schema string) (string, error) {
	if !validSchemaName(schema) {
		return "", fmt.Errorf("refusing to build DSN for invalid schema %q", schema)
	}
	if !strings.Contains(dsn, "://") {
		return "", fmt.Errorf("schema isolation requires a URL-form TEST_DATABASE_URL, got %q", dsn)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	q := url.Values{}
	q.Set("options", "-c search_path="+schema+",public")
	return dsn + sep + q.Encode(), nil
}

// provisionSchema creates the schema and applies every migration into it.
func provisionSchema(ctx context.Context, dsn, schema string) error {
	if !validSchemaName(schema) {
		return fmt.Errorf("invalid schema name %q", schema)
	}
	if err := validateTestDSN(dsn); err != nil {
		return err
	}

	admin, err := db.NewPool(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect admin pool: %w", err)
	}
	defer admin.Close()

	// Plain CREATE SCHEMA (not IF NOT EXISTS) so a collision with another
	// concurrently running test process fails loudly instead of sharing state.
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		return fmt.Errorf("create schema %s: %w", schema, err)
	}

	schemaURL, err := schemaDSN(dsn, schema)
	if err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, schemaURL)
	if err != nil {
		return fmt.Errorf("connect schema pool: %w", err)
	}
	defer pool.Close()

	return applyMigrations(ctx, pool)
}

// applyMigrations runs every .up.sql migration in filename order. pgx uses the
// simple query protocol when no arguments are supplied, so multi-statement
// migration files execute as-is.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}
