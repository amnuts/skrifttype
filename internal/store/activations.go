package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// ActivationKind identifies the kind of source claiming a font is
// active. The values are stored in the activations.source_kind column.
type ActivationKind = string

const (
	// ActivationKindManual identifies a font activated directly by the
	// user (single-font toggle).
	ActivationKindManual ActivationKind = "manual"

	// ActivationKindCollection identifies a font activated as a member
	// of an active collection. The SourceID field of ActivationSource
	// holds the collection ID.
	ActivationKindCollection ActivationKind = "collection"
)

// ActivationSource is one (kind, source_id) tuple. For manual
// activations SourceID.Valid is false; for collection activations
// SourceID.Valid is true and Int64 is the collection ID.
type ActivationSource struct {
	Kind     ActivationKind
	SourceID sql.NullInt64
}

// ActivationDiff records the change in the *global* set of active
// fonts produced by a ReplaceActivationSet call. NewlyActive lists
// fonts that gained their first source; NewlyInactive lists fonts
// that lost their last source. Order is ascending font_id.
type ActivationDiff struct {
	NewlyActive   []int64
	NewlyInactive []int64
}

// AddActivationSource inserts one activation row. Returns true if this
// row brought the font from inactive to active (i.e., there was no
// other source). Re-adding an existing source is a no-op and returns
// false (no state change).
func (s *Store) AddActivationSource(ctx context.Context, fontID int64, src ActivationSource) (becameActive bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	wasActive, err := isActiveTx(ctx, tx, fontID)
	if err != nil {
		return false, fmt.Errorf("pre-check: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO activations (font_id, source_kind, source_id, activated_at)
		 VALUES (?, ?, ?, ?)`,
		fontID, src.Kind, src.SourceID, nowUnix(),
	); err != nil {
		return false, fmt.Errorf("insert activation: %w", err)
	}

	nowActive, err := isActiveTx(ctx, tx, fontID)
	if err != nil {
		return false, fmt.Errorf("post-check: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return !wasActive && nowActive, nil
}

// RemoveActivationSource deletes the matching row. Returns true if
// removing this row brought the font from active to inactive (i.e.,
// no other source remains). Removing a row that doesn't exist is a
// no-op and returns false.
func (s *Store) RemoveActivationSource(ctx context.Context, fontID int64, src ActivationSource) (becameInactive bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	wasActive, err := isActiveTx(ctx, tx, fontID)
	if err != nil {
		return false, fmt.Errorf("pre-check: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM activations
		 WHERE font_id = ? AND source_kind = ? AND COALESCE(source_id, 0) = COALESCE(?, 0)`,
		fontID, src.Kind, src.SourceID,
	); err != nil {
		return false, fmt.Errorf("delete activation: %w", err)
	}

	nowActive, err := isActiveTx(ctx, tx, fontID)
	if err != nil {
		return false, fmt.Errorf("post-check: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return wasActive && !nowActive, nil
}

// ReplaceActivationSet atomically replaces all activations rows
// matching (src.Kind, src.SourceID) with rows for the given fontIDs.
// Returns the diff in the GLOBAL active-font set as a result of this
// replacement.
//
// Use this for collection activate/deactivate (pass the collection's
// full member list to activate; pass nil to deactivate). For
// single-font manual toggles, use AddActivationSource /
// RemoveActivationSource directly.
func (s *Store) ReplaceActivationSet(ctx context.Context, src ActivationSource, fontIDs []int64) (ActivationDiff, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := readDistinctActiveFontIDs(ctx, tx)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("read before: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM activations
		 WHERE source_kind = ? AND COALESCE(source_id, 0) = COALESCE(?, 0)`,
		src.Kind, src.SourceID,
	); err != nil {
		return ActivationDiff{}, fmt.Errorf("clear source: %w", err)
	}

	now := nowUnix()
	for _, fid := range fontIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO activations (font_id, source_kind, source_id, activated_at)
			 VALUES (?, ?, ?, ?)`,
			fid, src.Kind, src.SourceID, now,
		); err != nil {
			return ActivationDiff{}, fmt.Errorf("insert activation: %w", err)
		}
	}

	after, err := readDistinctActiveFontIDs(ctx, tx)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("read after: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ActivationDiff{}, fmt.Errorf("commit: %w", err)
	}

	return computeActivationDiff(before, after), nil
}

// ClearActivations removes every activation source from each font in one
// transaction and returns the resulting change to the global active set.
func (s *Store) ClearActivations(ctx context.Context, fontIDs []int64) (ActivationDiff, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := readDistinctActiveFontIDs(ctx, tx)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("read before: %w", err)
	}
	for _, id := range fontIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM activations WHERE font_id = ?`, id); err != nil {
			return ActivationDiff{}, fmt.Errorf("clear activations: %w", err)
		}
	}
	after, err := readDistinctActiveFontIDs(ctx, tx)
	if err != nil {
		return ActivationDiff{}, fmt.Errorf("read after: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ActivationDiff{}, fmt.Errorf("commit: %w", err)
	}
	return computeActivationDiff(before, after), nil
}

// IsActive returns true if the font has at least one activation source.
func (s *Store) IsActive(ctx context.Context, fontID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM activations WHERE font_id = ?`, fontID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("is active: %w", err)
	}
	return n > 0, nil
}

// ListActiveFontIDs returns the IDs of all currently-active fonts in
// ascending order.
func (s *Store) ListActiveFontIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT font_id FROM activations ORDER BY font_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListActivationSourcesForFont returns the activation sources currently
// keeping the font active, ordered by (kind, source_id).
func (s *Store) ListActivationSourcesForFont(ctx context.Context, fontID int64) ([]ActivationSource, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source_kind, source_id FROM activations
		 WHERE font_id = ? ORDER BY source_kind, COALESCE(source_id, 0)`,
		fontID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer rows.Close()
	var out []ActivationSource
	for rows.Next() {
		var src ActivationSource
		if err := rows.Scan(&src.Kind, &src.SourceID); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// isActiveTx is the within-transaction equivalent of IsActive.
func isActiveTx(ctx context.Context, tx *sql.Tx, fontID int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM activations WHERE font_id = ?`, fontID,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// readDistinctActiveFontIDs is the within-transaction equivalent of
// ListActiveFontIDs.
func readDistinctActiveFontIDs(ctx context.Context, tx *sql.Tx) (map[int64]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT font_id FROM activations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// computeActivationDiff returns the symmetric difference of two
// distinct-font-id sets: NewlyActive = after \ before, NewlyInactive =
// before \ after. Output slices are sorted ascending.
func computeActivationDiff(before, after map[int64]struct{}) ActivationDiff {
	var diff ActivationDiff
	for id := range after {
		if _, ok := before[id]; !ok {
			diff.NewlyActive = append(diff.NewlyActive, id)
		}
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			diff.NewlyInactive = append(diff.NewlyInactive, id)
		}
	}
	sort.Slice(diff.NewlyActive, func(i, j int) bool { return diff.NewlyActive[i] < diff.NewlyActive[j] })
	sort.Slice(diff.NewlyInactive, func(i, j int) bool { return diff.NewlyInactive[i] < diff.NewlyInactive[j] })
	return diff
}

// ActivationRow is one row of the activations table.
type ActivationRow struct {
	FontID int64
	Source ActivationSource
}

// ListActivations returns every activation row, ordered by font then
// source. Used to build the per-font source map the UI displays.
func (s *Store) ListActivations(ctx context.Context) ([]ActivationRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT font_id, source_kind, source_id FROM activations
		 ORDER BY font_id, source_kind, COALESCE(source_id, 0)`,
	)
	if err != nil {
		return nil, fmt.Errorf("list activations: %w", err)
	}
	defer rows.Close()
	var out []ActivationRow
	for rows.Next() {
		var r ActivationRow
		if err := rows.Scan(&r.FontID, &r.Source.Kind, &r.Source.SourceID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFontIDsForSource returns the fonts held active by exactly this
// source, in ascending order.
func (s *Store) ListFontIDsForSource(ctx context.Context, src ActivationSource) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT font_id FROM activations
		 WHERE source_kind = ? AND COALESCE(source_id, 0) = COALESCE(?, 0)
		 ORDER BY font_id`,
		src.Kind, src.SourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list fonts for source: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListLibraryPaths returns every distinct file path of a library font,
// whether or not it is available or currently active.
func (s *Store) ListLibraryPaths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT path FROM fonts WHERE source = ? ORDER BY path`, FontSourceLibrary)
	if err != nil {
		return nil, fmt.Errorf("list library paths: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListLoadableActivePaths returns font_id -> path for every active font
// that the activation layer should actually have loaded: library fonts
// whose file is available and whose watched root (if any) is enabled.
// System fonts and missing files are excluded (spec §5 invariants 4/5), as
// are WOFF/WOFF2 web fonts, which GDI cannot load: this is the single
// source of paths for reconcile, so a stale activation row for a web font
// never reaches AddFontResource.
func (s *Store) ListLoadableActivePaths(ctx context.Context) (map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT f.id, f.path
FROM activations a
JOIN fonts f ON f.id = a.font_id
LEFT JOIN watched_roots w ON w.id = f.watched_root_id
WHERE f.source = ? AND f.available = 1 AND f.format NOT IN ('woff', 'woff2') AND (w.id IS NULL OR w.enabled = 1)`,
		FontSourceLibrary,
	)
	if err != nil {
		return nil, fmt.Errorf("list loadable active paths: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[id] = path
	}
	return out, rows.Err()
}
