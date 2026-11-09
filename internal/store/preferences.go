package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetPreference returns the value for key and whether it existed.
func (s *Store) GetPreference(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM preferences WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get preference %q: %w", key, err)
	}
	return v, true, nil
}

// SetPreference upserts key/value into the preferences table.
func (s *Store) SetPreference(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO preferences (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set preference %q: %w", key, err)
	}
	return nil
}
