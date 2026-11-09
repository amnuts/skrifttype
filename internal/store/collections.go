package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Collection is a user-defined group of fonts. Membership is many-to-many
// via collection_members.
type Collection struct {
	ID        int64
	Name      string
	Color     string // optional, "" means unset
	SortOrder int
	CreatedAt int64 // unix seconds
	Active    bool  // user has switched the collection on
}

// CreateCollection inserts a new empty collection. Fails on duplicate
// name (UNIQUE constraint).
func (s *Store) CreateCollection(ctx context.Context, name string) (Collection, error) {
	now := nowUnix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO collections (name, color, sort_order, created_at) VALUES (?, NULL, 0, ?)`,
		name, now,
	)
	if err != nil {
		return Collection{}, fmt.Errorf("insert collection: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Collection{}, fmt.Errorf("LastInsertId: %w", err)
	}
	return Collection{ID: id, Name: name, CreatedAt: now}, nil
}

// DeleteCollection removes the collection and (via ON DELETE CASCADE)
// all of its membership rows.
func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

// RenameCollection updates the name. Fails on UNIQUE conflict.
func (s *Store) RenameCollection(ctx context.Context, id int64, newName string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE collections SET name = ? WHERE id = ?`, newName, id)
	if err != nil {
		return fmt.Errorf("rename collection: %w", err)
	}
	return nil
}

// GetCollection fetches one collection by ID.
func (s *Store) GetCollection(ctx context.Context, id int64) (Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, color, sort_order, created_at, active FROM collections WHERE id = ?`, id,
	)
	return scanCollection(row)
}

// ListCollections returns all collections ordered by sort_order then ID.
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, color, sort_order, created_at, active FROM collections ORDER BY sort_order, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddFontToCollection inserts a membership row. Idempotent: re-adding
// an existing member is a no-op (uses INSERT OR IGNORE).
func (s *Store) AddFontToCollection(ctx context.Context, collectionID, fontID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO collection_members (collection_id, font_id) VALUES (?, ?)`,
		collectionID, fontID,
	)
	if err != nil {
		return fmt.Errorf("add font to collection: %w", err)
	}
	return nil
}

// RemoveFontFromCollection deletes a membership row. Idempotent:
// removing a non-member is not an error.
func (s *Store) RemoveFontFromCollection(ctx context.Context, collectionID, fontID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM collection_members WHERE collection_id = ? AND font_id = ?`,
		collectionID, fontID,
	)
	if err != nil {
		return fmt.Errorf("remove font from collection: %w", err)
	}
	return nil
}

// ListFontsInCollection returns the IDs of fonts in this collection,
// in ascending font-id order. Callers fetch full font rows via
// GetFontByID as needed.
func (s *Store) ListFontsInCollection(ctx context.Context, collectionID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT font_id FROM collection_members WHERE collection_id = ? ORDER BY font_id`,
		collectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list fonts in collection: %w", err)
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

// ListCollectionsForFont returns collection IDs that contain this font.
func (s *Store) ListCollectionsForFont(ctx context.Context, fontID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT collection_id FROM collection_members WHERE font_id = ? ORDER BY collection_id`,
		fontID,
	)
	if err != nil {
		return nil, fmt.Errorf("list collections for font: %w", err)
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

func scanCollection(r rowScanner) (Collection, error) {
	var c Collection
	var color sql.NullString
	var active int
	if err := r.Scan(&c.ID, &c.Name, &color, &c.SortOrder, &c.CreatedAt, &active); err != nil {
		return Collection{}, err
	}
	c.Color = color.String
	c.Active = active != 0
	return c, nil
}

// SetCollectionColor updates the display colour ("" clears it).
func (s *Store) SetCollectionColor(ctx context.Context, id int64, color string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE collections SET color = ? WHERE id = ?`, nullableString(color), id)
	if err != nil {
		return fmt.Errorf("set collection color: %w", err)
	}
	return nil
}

// SetCollectionActive records whether the user has switched the
// collection on. It does not touch the activations table — the
// activation manager owns that.
func (s *Store) SetCollectionActive(ctx context.Context, id int64, active bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE collections SET active = ? WHERE id = ?`, boolToInt(active), id)
	if err != nil {
		return fmt.Errorf("set collection active: %w", err)
	}
	return nil
}
