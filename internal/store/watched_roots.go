package store

import (
	"context"
	"fmt"
)

// WatchedRoot is one user-configured directory that the library
// scanner watches for fonts.
type WatchedRoot struct {
	ID        int64
	Path      string // absolute path, caller's responsibility to canonicalise
	Recursive bool
	Enabled   bool
	AddedAt   int64 // unix seconds
}

// AddWatchedRoot inserts a new watched root and returns it with the
// assigned ID. Fails if the path is already watched (UNIQUE constraint).
func (s *Store) AddWatchedRoot(ctx context.Context, path string, recursive bool) (WatchedRoot, error) {
	now := nowUnix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO watched_roots (path, recursive, enabled, added_at)
		 VALUES (?, ?, 1, ?)`,
		path, boolToInt(recursive), now,
	)
	if err != nil {
		return WatchedRoot{}, fmt.Errorf("insert watched_root: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return WatchedRoot{}, fmt.Errorf("LastInsertId: %w", err)
	}
	return WatchedRoot{
		ID:        id,
		Path:      path,
		Recursive: recursive,
		Enabled:   true,
		AddedAt:   now,
	}, nil
}

// RemoveWatchedRoot deletes the watched root by ID. Fonts referencing
// this root via watched_root_id are not deleted; their FK is set NULL
// per the schema.
func (s *Store) RemoveWatchedRoot(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watched_roots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete watched_root: %w", err)
	}
	return nil
}

// SetWatchedRootEnabled toggles the enabled flag.
func (s *Store) SetWatchedRootEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE watched_roots SET enabled = ? WHERE id = ?`,
		boolToInt(enabled), id,
	)
	if err != nil {
		return fmt.Errorf("update watched_root enabled: %w", err)
	}
	return nil
}

// ListWatchedRoots returns all watched roots, including disabled ones,
// ordered by added_at ascending (insertion order).
func (s *Store) ListWatchedRoots(ctx context.Context) ([]WatchedRoot, error) {
	return s.queryWatchedRoots(ctx, `SELECT id, path, recursive, enabled, added_at FROM watched_roots ORDER BY added_at`)
}

// ListEnabledWatchedRoots returns only enabled roots — the set the
// library scanner should walk on its next pass.
func (s *Store) ListEnabledWatchedRoots(ctx context.Context) ([]WatchedRoot, error) {
	return s.queryWatchedRoots(ctx, `SELECT id, path, recursive, enabled, added_at FROM watched_roots WHERE enabled = 1 ORDER BY added_at`)
}

func (s *Store) queryWatchedRoots(ctx context.Context, query string) ([]WatchedRoot, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query watched_roots: %w", err)
	}
	defer rows.Close()

	var out []WatchedRoot
	for rows.Next() {
		var r WatchedRoot
		var rec, en int
		if err := rows.Scan(&r.ID, &r.Path, &rec, &en, &r.AddedAt); err != nil {
			return nil, fmt.Errorf("scan watched_root: %w", err)
		}
		r.Recursive = rec != 0
		r.Enabled = en != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// boolToInt converts a Go bool to the SQLite integer representation
// (1 or 0). Centralised so the convention stays consistent.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
