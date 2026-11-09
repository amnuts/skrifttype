package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMigration0003PreservesRelations(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "mig.db")
	connDSN := dsn + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", connDSN)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := runMigrationsTo(ctx, db, 2); err != nil {
		t.Fatalf("migrate to 2: %v", err)
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO watched_roots (id, path, added_at) VALUES (1, 'D:\fonts', 1)`)
	for i, fam := range []string{"Alpha Sans", "Bravo Serif", "Charlie Mono"} {
		mustExec(`INSERT INTO fonts (id, path, file_size, file_mtime, family, subfamily, full_name,
			format, source, watched_root_id, first_seen_at, last_seen_at, designer)
			VALUES (?, ?, 1, 1, ?, 'Regular', ?, 'ttf', 'library', 1, 1, 1, 'Zed Designer')`,
			10+i, `D:\fonts\f`+fam+`.ttf`, fam, fam+" Regular")
	}
	mustExec(`INSERT INTO collections (id, name, created_at) VALUES (5, 'Mine', 1)`)
	mustExec(`INSERT INTO collection_members (collection_id, font_id) VALUES (5, 10), (5, 12)`)
	mustExec(`INSERT INTO activations (font_id, source_kind, source_id, activated_at) VALUES (11, 'font', NULL, 1), (12, 'collection', 5, 1)`)
	mustExec(`INSERT INTO tags (id, name) VALUES (1, 'x')`)
	mustExec(`INSERT INTO font_tags (font_id, tag_id) VALUES (10, 1)`)

	tables := []string{"fonts", "collection_members", "activations", "font_tags"}
	before := map[string]int{}
	for _, tb := range tables {
		before[tb] = countRows(t, db, tb)
	}

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	for _, tb := range tables {
		if got := countRows(t, db, tb); got != before[tb] {
			t.Errorf("%s: %d rows before, %d after", tb, before[tb], got)
		}
	}
	var ids string
	if err := db.QueryRow(`SELECT group_concat(id) FROM (SELECT id FROM fonts ORDER BY id)`).Scan(&ids); err != nil {
		t.Fatal(err)
	}
	if ids != "10,11,12" {
		t.Errorf("font ids: got %q, want 10,11,12", ids)
	}
	var member int
	if err := db.QueryRow(`SELECT COUNT(*) FROM collection_members WHERE collection_id=5 AND font_id IN (10,12)`).Scan(&member); err != nil || member != 2 {
		t.Errorf("collection members: %d, %v", member, err)
	}

	// FTS finds rows (family and designer columns), via the rebuilt index.
	var hit int64
	if err := db.QueryRow(`SELECT rowid FROM fonts_fts WHERE fonts_fts MATCH 'Bravo'`).Scan(&hit); err != nil || hit != 11 {
		t.Errorf("fts Bravo: rowid %d, err %v", hit, err)
	}
	if n := func() int {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM fonts_fts WHERE fonts_fts MATCH 'Zed'`).Scan(&n)
		return n
	}(); n != 3 {
		t.Errorf("fts Zed: %d hits, want 3", n)
	}

	// Triggers still work after the rebuild.
	mustExec(`INSERT INTO fonts (id, path, file_size, file_mtime, family, subfamily, full_name,
		format, face_index, source, first_seen_at, last_seen_at)
		VALUES (20, 'D:\fonts\c.ttc', 1, 1, 'Delta Face', 'Regular', 'Delta', 'ttc', 0, 'library', 1, 1),
		       (21, 'D:\fonts\c.ttc', 1, 1, 'Echo Face', 'Regular', 'Echo', 'ttc', 1, 'library', 1, 1)`)
	if err := db.QueryRow(`SELECT rowid FROM fonts_fts WHERE fonts_fts MATCH 'Echo'`).Scan(&hit); err != nil || hit != 21 {
		t.Errorf("fts after insert: %d, %v", hit, err)
	}
	if _, err := db.Exec(`INSERT INTO fonts (path, file_size, file_mtime, family, subfamily, full_name,
		format, face_index, source, first_seen_at, last_seen_at)
		VALUES ('D:\fonts\c.ttc', 1, 1, 'Dup', 'R', 'Dup', 'ttc', 1, 'library', 1, 1)`); err == nil {
		t.Error("expected UNIQUE(path, face_index) violation")
	}

	// foreign_keys must be back ON for the pooled connection.
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys after migration: %d, %v", fk, err)
	}
	var fkRows int
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		fkRows++
	}
	rows.Close()
	if fkRows != 0 {
		t.Errorf("foreign_key_check reported %d violations", fkRows)
	}
}
