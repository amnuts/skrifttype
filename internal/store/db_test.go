package store_test

import (
	"context"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

// latestSchemaVersion is the highest NNNN_ prefix under migrations/.
// Bump it when adding a migration.
const latestSchemaVersion = 3

func TestOpen_AppliesAllMigrations(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Every migration must have been applied after Open.
	var version int
	if err := s.DB().QueryRowContext(ctx, `SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != latestSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", latestSchemaVersion, version)
	}

	// Spot-check that the major tables and the FTS virtual table all exist.
	want := []string{
		"watched_roots", "fonts", "fonts_fts", "collections",
		"collection_members", "activations", "tags", "font_tags", "preferences",
	}
	for _, name := range want {
		var n int
		err := s.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE name = ?`,
			name,
		).Scan(&n)
		if err != nil {
			t.Fatalf("checking %s: %v", name, err)
		}
		if n == 0 {
			t.Fatalf("expected table %s to exist after Open", name)
		}
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Re-running migrations on an already-migrated DB must be a no-op.
	// (Calls the unexported runMigrations indirectly: we re-Open by reusing
	// the same DSN — but :memory: is per-connection. Instead, we exercise
	// idempotency by directly re-applying via the same Store: there must
	// be exactly one schema_version row per migration after re-running.)
	if err := s.RerunMigrations(ctx); err != nil {
		t.Fatalf("RerunMigrations: %v", err)
	}

	var rows int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_version`,
	).Scan(&rows); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if rows != latestSchemaVersion {
		t.Fatalf("expected %d schema_version rows after re-run, got %d", latestSchemaVersion, rows)
	}
}
