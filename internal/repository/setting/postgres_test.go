package setting

import (
	"context"
	"testing"

	"github.com/deeptrols/api/internal/repository/testutil"
)

func TestUpsertGetAll(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped (no TEST_DATABASE_URL)
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "system_settings")

	if err := repo.Upsert(ctx, []Entry{{Key: "site_name", Value: []byte(`"Acme"`)}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.Get(ctx, "site_name")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 || string(got[0].Value) != `"Acme"` {
		t.Fatalf("unexpected Get result: %+v", got)
	}

	// Same key upsert overwrites the row rather than duplicating it.
	if err := repo.Upsert(ctx, []Entry{{Key: "site_name", Value: []byte(`"Acme2"`)}}); err != nil {
		t.Fatalf("Upsert2: %v", err)
	}
	all, err := repo.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	count := 0
	for _, e := range all {
		if e.Key == "site_name" {
			if string(e.Value) != `"Acme2"` {
				t.Fatalf("expected overwritten value, got %s", e.Value)
			}
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row for key, got %d", count)
	}
}
