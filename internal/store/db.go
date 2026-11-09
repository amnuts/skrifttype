// Package store is the SQLite-backed persistence layer for skrifttype.
// It owns the schema (defined in migrations/) and exposes typed
// operations for fonts, watched roots, collections, and activations.
//
// The package is platform-neutral. SQLite is provided by
// modernc.org/sqlite (pure Go, no CGo). FTS5 is enabled by default in
// that build.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// migrationsFS embeds every *.sql file under migrations/ at build time.
// Filenames must follow NNNN_description.sql with a zero-padded version
// prefix; the runner sorts lexicographically.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps a *sql.DB plus the schema discipline this package owns.
// All operations on the database go through methods on this type.
type Store struct {
	db *sql.DB
}

// Open creates or opens a SQLite database at the given DSN, applies any
// pending migrations, and returns a Store. The DSN may be ":memory:"
// for tests or an absolute file path for production. Pragmas for
// foreign-key enforcement and WAL journaling are enabled.
func Open(ctx context.Context, dsn string) (*Store, error) {
	connDSN := dsn
	if !strings.Contains(connDSN, "?") {
		connDSN += "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", connDSN)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// :memory: databases are per-connection; SetMaxOpenConns(1) keeps
	// the test DSN single-connection so the schema we apply is the
	// schema subsequent queries see. For file DSNs the same setting
	// is conservative but harmless given our query volumes.
	db.SetMaxOpenConns(1)

	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("runMigrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying *sql.DB.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB. Used by tests and by future
// packages that need raw access; production code in this package
// should prefer the typed methods.
func (s *Store) DB() *sql.DB { return s.db }

// RerunMigrations re-applies any not-yet-applied migrations. Used by
// tests to assert idempotency; production callers do not need to call
// this — Open already runs migrations.
func (s *Store) RerunMigrations(ctx context.Context) error {
	return runMigrations(ctx, s.db)
}

// nowUnix returns the current time as unix seconds. Centralised so
// tests can swap it via a future override hook if needed.
func nowUnix() int64 { return time.Now().Unix() }

func runMigrations(ctx context.Context, db *sql.DB) error {
	return runMigrationsTo(ctx, db, 0)
}

// runMigrationsTo applies pending migrations up to and including
// maxVersion; maxVersion <= 0 means no limit. Tests use the limit to
// build a database at an older schema version.
func runMigrationsTo(ctx context.Context, db *sql.DB, maxVersion int) error {
	current, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("currentSchemaVersion: %w", err)
	}

	files, err := loadMigrationFiles()
	if err != nil {
		return fmt.Errorf("loadMigrationFiles: %w", err)
	}

	for _, m := range files {
		if m.version <= current {
			continue
		}
		if maxVersion > 0 && m.version > maxVersion {
			break
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("applying migration %d: %w", m.version, err)
		}
	}
	return nil
}

// currentSchemaVersion reads MAX(version) from the schema_version
// table. If the table does not exist (fresh DB), version 0 is returned
// without error.
func currentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var has int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`,
	).Scan(&has)
	if err != nil {
		return 0, err
	}
	if has == 0 {
		return 0, nil
	}
	var v sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

type migrationFile struct {
	version int
	name    string
	body    string
}

func loadMigrationFiles() ([]migrationFile, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		ver, err := parseVersion(e.Name())
		if err != nil {
			return nil, fmt.Errorf("migration filename %q: %w", e.Name(), err)
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migrationFile{
			version: ver,
			name:    e.Name(),
			body:    string(body),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// parseVersion extracts the leading numeric prefix from a filename
// like "0001_init.sql".
func parseVersion(name string) (int, error) {
	i := strings.IndexByte(name, '_')
	if i <= 0 {
		return 0, fmt.Errorf("expected NNNN_description.sql, got %q", name)
	}
	return strconv.Atoi(name[:i])
}

// foreignKeysOffDirective, as the first line of a migration, makes the
// runner apply it with foreign key enforcement disabled (needed to
// rebuild a table that other tables reference without cascading).
const foreignKeysOffDirective = "-- migrate:foreign-keys-off"

func applyMigration(ctx context.Context, db *sql.DB, m migrationFile) error {
	if firstLine, _, _ := strings.Cut(m.body, "\n"); strings.TrimSpace(firstLine) == foreignKeysOffDirective {
		return applyMigrationFKOff(ctx, db, m)
	}
	return applyMigrationOn(ctx, db, m, nil)
}

// applyMigrationFKOff runs m on a dedicated connection with
// PRAGMA foreign_keys=OFF. The pragma is a no-op inside a transaction,
// so it must be set before BEGIN. After the body, foreign_key_check
// must report nothing or the migration is rolled back. The connection's
// original foreign_keys setting is restored on every exit path; the
// pool is single-connection, so that is the connection later queries
// use.
func applyMigrationFKOff(ctx context.Context, db *sql.DB, m migrationFile) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	var prev int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&prev); err != nil {
		return fmt.Errorf("read foreign_keys: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}
	defer func() {
		// Use a fresh context so restoration happens even if ctx is cancelled.
		if _, rerr := conn.ExecContext(context.Background(),
			fmt.Sprintf(`PRAGMA foreign_keys=%d`, prev)); rerr != nil && err == nil {
			err = fmt.Errorf("restore foreign_keys: %w", rerr)
		}
	}()

	return applyMigrationOn(ctx, conn, m, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			return fmt.Errorf("foreign_key_check: %w", err)
		}
		defer rows.Close()
		if rows.Next() {
			return fmt.Errorf("foreign_key_check reported violations after migration")
		}
		return rows.Err()
	})
}

// txBeginner is satisfied by both *sql.DB and *sql.Conn.
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// applyMigrationOn runs the migration body and records its version in
// one transaction. verify, if non-nil, runs before commit; an error
// from it rolls the migration back.
func applyMigrationOn(ctx context.Context, b txBeginner, m migrationFile, verify func(*sql.Tx) error) error {
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.body); err != nil {
		return fmt.Errorf("exec body: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
		m.version, nowUnix(),
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	if verify != nil {
		if err := verify(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
