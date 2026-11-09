package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// FontSource identifies how a font row entered the library. The values
// are stored in the fonts.source TEXT column. This enum intentionally
// duplicates the winfont.Source values rather than depending on that
// package — winfont is the read-side enumerator, store is the
// write-side persistence; the two should not couple.
type FontSource = string

const (
	FontSourceLibrary    FontSource = "library"
	FontSourceSystemHKLM FontSource = "system-hklm"
	FontSourceSystemHKCU FontSource = "system-hkcu"
)

// CodepointRange is one inclusive [Lo, Hi] range in the unicode_codepoints
// JSON column. Used by the glyph-map UI.
type CodepointRange struct {
	Lo int `json:"lo"`
	Hi int `json:"hi"`
}

// VariationAxis is one entry in the variation_axes JSON column.
// Populated for fonts with an OpenType fvar table.
type VariationAxis struct {
	Tag  string  `json:"tag"`
	Name string  `json:"name"`
	Min  float64 `json:"min"`
	Def  float64 `json:"def"`
	Max  float64 `json:"max"`
}

// Font is one row in the fonts table. All path values are absolute;
// the store does not canonicalise on the way in. Optional string
// columns use empty string as the zero value rather than sql.NullString.
//
// JSON-backed slice fields (GsubFeatures, GposFeatures, VariationAxes,
// UnicodeRanges, UnicodeCodepoints) round-trip with one quirk: a nil
// slice on insert decodes back as a non-nil zero-length slice on read,
// because the underlying TEXT column always stores "[]". Treat nil and
// length-zero slices as equivalent.
type Font struct {
	ID                int64
	Path              string
	FileSize          int64
	FileMtime         int64 // unix seconds
	SHA256            string
	Family            string
	Subfamily         string
	FullName          string
	PostscriptName    string
	Format            string
	FaceIndex         int
	WeightClass       int // OS/2 usWeightClass; 0 means unknown
	WidthClass        int // OS/2 usWidthClass; 0 means unknown
	Italic            bool
	IsVariable        bool
	Classification    string // "serif"|"sans"|"mono"|"script"|"display"|""
	Designer          string
	Foundry           string
	LicenseText       string
	VersionString     string
	GsubFeatures      []string
	GposFeatures      []string
	VariationAxes     []VariationAxis
	UnicodeRanges     []string
	UnicodeCodepoints []CodepointRange
	ParseError        string
	Source            FontSource
	WatchedRootID     int64 // 0 means NULL (no watched root, e.g. system font)
	FirstSeenAt       int64 // unix seconds
	LastSeenAt        int64 // unix seconds
	Available         bool
}

// FontFilter narrows ListFonts queries.
type FontFilter struct {
	Source         FontSource // empty = all sources
	Classification string     // empty = all classifications
	AvailableOnly  bool       // if true, exclude rows with available=0
	Limit          int        // 0 = no limit
	Offset         int
}

// UpsertFont inserts a new row or updates the existing row matched by
// path. Returns the row ID. Sets first_seen_at on insert and updates
// last_seen_at on every call.
func (s *Store) UpsertFont(ctx context.Context, f Font) (int64, error) {
	gsubJSON, err := marshalJSON(f.GsubFeatures)
	if err != nil {
		return 0, fmt.Errorf("marshal gsub: %w", err)
	}
	gposJSON, err := marshalJSON(f.GposFeatures)
	if err != nil {
		return 0, fmt.Errorf("marshal gpos: %w", err)
	}
	axesJSON, err := marshalJSON(f.VariationAxes)
	if err != nil {
		return 0, fmt.Errorf("marshal axes: %w", err)
	}
	rangesJSON, err := marshalJSON(f.UnicodeRanges)
	if err != nil {
		return 0, fmt.Errorf("marshal ranges: %w", err)
	}
	codepointsJSON, err := marshalJSON(f.UnicodeCodepoints)
	if err != nil {
		return 0, fmt.Errorf("marshal codepoints: %w", err)
	}

	now := nowUnix()
	wri := nullableInt64(f.WatchedRootID)
	wc := nullableInt64(int64(f.WeightClass))
	wd := nullableInt64(int64(f.WidthClass))
	cls := nullableString(f.Classification)
	psn := nullableString(f.PostscriptName)
	sha := nullableString(f.SHA256)
	des := nullableString(f.Designer)
	fnd := nullableString(f.Foundry)
	lic := nullableString(f.LicenseText)
	ver := nullableString(f.VersionString)
	pe := nullableString(f.ParseError)

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO fonts (
    path, file_size, file_mtime, sha256,
    family, subfamily, full_name, postscript_name, format, face_index,
    weight_class, width_class, italic, is_variable, classification,
    designer, foundry, license_text, version_string,
    gsub_features, gpos_features, variation_axes, unicode_ranges, unicode_codepoints,
    parse_error, source, watched_root_id, first_seen_at, last_seen_at, available
)
VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?
)
ON CONFLICT(path, face_index) DO UPDATE SET
    file_size          = excluded.file_size,
    file_mtime         = excluded.file_mtime,
    sha256             = excluded.sha256,
    family             = excluded.family,
    subfamily          = excluded.subfamily,
    full_name          = excluded.full_name,
    postscript_name    = excluded.postscript_name,
    format             = excluded.format,
    weight_class       = excluded.weight_class,
    width_class        = excluded.width_class,
    italic             = excluded.italic,
    is_variable        = excluded.is_variable,
    classification     = excluded.classification,
    designer           = excluded.designer,
    foundry            = excluded.foundry,
    license_text       = excluded.license_text,
    version_string     = excluded.version_string,
    gsub_features      = excluded.gsub_features,
    gpos_features      = excluded.gpos_features,
    variation_axes     = excluded.variation_axes,
    unicode_ranges     = excluded.unicode_ranges,
    unicode_codepoints = excluded.unicode_codepoints,
    parse_error        = excluded.parse_error,
    source             = excluded.source,
    watched_root_id    = excluded.watched_root_id,
    last_seen_at       = excluded.last_seen_at,
    available          = excluded.available
`,
		f.Path, f.FileSize, f.FileMtime, sha,
		f.Family, f.Subfamily, f.FullName, psn, f.Format, f.FaceIndex,
		wc, wd, boolToInt(f.Italic), boolToInt(f.IsVariable), cls,
		des, fnd, lic, ver,
		gsubJSON, gposJSON, axesJSON, rangesJSON, codepointsJSON,
		pe, f.Source, wri, now, now, boolToInt(f.Available),
	); err != nil {
		return 0, fmt.Errorf("upsert font: %w", err)
	}

	// LastInsertId on UPSERT-via-ON-CONFLICT may not return the existing
	// row's ID on an UPDATE branch in all SQLite versions. Always fetch
	// by (path, face_index) to be safe.
	id, err := s.fontIDByPathFace(ctx, f.Path, f.FaceIndex)
	if err != nil {
		return 0, fmt.Errorf("read back id: %w", err)
	}
	return id, nil
}

func (s *Store) fontIDByPathFace(ctx context.Context, path string, faceIndex int) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM fonts WHERE path = ? AND face_index = ?`, path, faceIndex).Scan(&id)
	return id, err
}

// GetFontByID fetches a single font row by ID. Returns sql.ErrNoRows
// (wrapped) if no row exists.
func (s *Store) GetFontByID(ctx context.Context, id int64) (Font, error) {
	row := s.db.QueryRowContext(ctx, fontSelectColumns()+` WHERE id = ?`, id)
	return scanFont(row)
}

// GetFontByPathFace fetches a single face of a font file. Returns
// sql.ErrNoRows (wrapped) if no row exists.
func (s *Store) GetFontByPathFace(ctx context.Context, path string, faceIndex int) (Font, error) {
	row := s.db.QueryRowContext(ctx, fontSelectColumns()+` WHERE path = ? AND face_index = ?`, path, faceIndex)
	return scanFont(row)
}

// ListFontsByPath returns every face stored for a file path, ordered by
// face_index. It returns an empty slice if the path is unknown.
func (s *Store) ListFontsByPath(ctx context.Context, path string) ([]Font, error) {
	rows, err := s.db.QueryContext(ctx, fontSelectColumns()+` WHERE path = ? ORDER BY face_index`, path)
	if err != nil {
		return nil, fmt.Errorf("list fonts by path: %w", err)
	}
	defer rows.Close()
	out := []Font{}
	for rows.Next() {
		f, err := scanFont(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListFonts returns rows matching the filter, ordered by family then
// subfamily for stable display.
func (s *Store) ListFonts(ctx context.Context, f FontFilter) ([]Font, error) {
	var b strings.Builder
	b.WriteString(fontSelectColumns())
	args := make([]interface{}, 0, 4)

	var conditions []string
	if f.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, f.Source)
	}
	if f.Classification != "" {
		conditions = append(conditions, "classification = ?")
		args = append(args, f.Classification)
	}
	if f.AvailableOnly {
		conditions = append(conditions, "available = 1")
	}
	if len(conditions) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(conditions, " AND "))
	}

	b.WriteString(" ORDER BY family, subfamily, id")

	if f.Limit > 0 {
		b.WriteString(" LIMIT ?")
		args = append(args, f.Limit)
		if f.Offset > 0 {
			b.WriteString(" OFFSET ?")
			args = append(args, f.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list fonts: %w", err)
	}
	defer rows.Close()

	var out []Font
	for rows.Next() {
		got, err := scanFont(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, rows.Err()
}

// DeleteFontsInWatchedRoot removes every font catalogued under the
// given watched root (and, via ON DELETE CASCADE, their activations and
// collection memberships). Call before removing the root itself —
// the FK is ON DELETE SET NULL, which would otherwise orphan them.
func (s *Store) DeleteFontsInWatchedRoot(ctx context.Context, rootID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM fonts WHERE watched_root_id = ?`, rootID)
	if err != nil {
		return fmt.Errorf("delete fonts in root: %w", err)
	}
	return nil
}

// DeleteFont removes the font and (via ON DELETE CASCADE) its
// activations and collection_members rows.
func (s *Store) DeleteFont(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM fonts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete font: %w", err)
	}
	return nil
}

// MarkFontUnavailable sets available=0 without otherwise modifying the
// row. Used by the library scanner when a previously-known file is
// missing from disk.
func (s *Store) MarkFontUnavailable(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE fonts SET available = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark font unavailable: %w", err)
	}
	return nil
}

// fontSelectColumns returns the SELECT clause and column list used
// everywhere this package needs to deserialize a Font.
func fontSelectColumns() string {
	return `SELECT
    id, path, file_size, file_mtime, sha256,
    family, subfamily, full_name, postscript_name, format, face_index,
    weight_class, width_class, italic, is_variable, classification,
    designer, foundry, license_text, version_string,
    gsub_features, gpos_features, variation_axes, unicode_ranges, unicode_codepoints,
    parse_error, source, watched_root_id, first_seen_at, last_seen_at, available
FROM fonts`
}

// scanFont reads one row from a query/queryRow. Works for both
// *sql.Row and *sql.Rows because both support Scan.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanFont(r rowScanner) (Font, error) {
	var f Font
	var sha, psn, classification, designer, foundry, license, version, parseErr sql.NullString
	var weightClass, widthClass, watchedRootID sql.NullInt64
	var italic, isVariable, available int
	var gsubJSON, gposJSON, axesJSON, rangesJSON, codepointsJSON string

	err := r.Scan(
		&f.ID, &f.Path, &f.FileSize, &f.FileMtime, &sha,
		&f.Family, &f.Subfamily, &f.FullName, &psn, &f.Format, &f.FaceIndex,
		&weightClass, &widthClass, &italic, &isVariable, &classification,
		&designer, &foundry, &license, &version,
		&gsubJSON, &gposJSON, &axesJSON, &rangesJSON, &codepointsJSON,
		&parseErr, &f.Source, &watchedRootID, &f.FirstSeenAt, &f.LastSeenAt, &available,
	)
	if err != nil {
		return Font{}, err
	}

	f.SHA256 = sha.String
	f.PostscriptName = psn.String
	f.Classification = classification.String
	f.Designer = designer.String
	f.Foundry = foundry.String
	f.LicenseText = license.String
	f.VersionString = version.String
	f.ParseError = parseErr.String
	if weightClass.Valid {
		f.WeightClass = int(weightClass.Int64)
	}
	if widthClass.Valid {
		f.WidthClass = int(widthClass.Int64)
	}
	if watchedRootID.Valid {
		f.WatchedRootID = watchedRootID.Int64
	}
	f.Italic = italic != 0
	f.IsVariable = isVariable != 0
	f.Available = available != 0

	if err := unmarshalJSON(gsubJSON, &f.GsubFeatures); err != nil {
		return Font{}, fmt.Errorf("gsub_features: %w", err)
	}
	if err := unmarshalJSON(gposJSON, &f.GposFeatures); err != nil {
		return Font{}, fmt.Errorf("gpos_features: %w", err)
	}
	if err := unmarshalJSON(axesJSON, &f.VariationAxes); err != nil {
		return Font{}, fmt.Errorf("variation_axes: %w", err)
	}
	if err := unmarshalJSON(rangesJSON, &f.UnicodeRanges); err != nil {
		return Font{}, fmt.Errorf("unicode_ranges: %w", err)
	}
	if err := unmarshalJSON(codepointsJSON, &f.UnicodeCodepoints); err != nil {
		return Font{}, fmt.Errorf("unicode_codepoints: %w", err)
	}
	return f, nil
}

// nullableString returns a sql.NullString that is .Valid only when s
// is non-empty. Empty Go strings map to SQL NULL.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt64 returns nil for 0 (mapping to SQL NULL) or the value.
func nullableInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// marshalJSON serializes v to a JSON string. Nil slices serialize as
// "[]" so the column never carries a SQL NULL even though the Go zero
// value is nil.
func marshalJSON(v interface{}) (string, error) {
	if v == nil {
		return "[]", nil
	}
	// reflect on pointer/slice types via json — empty/nil slices are
	// emitted as "null" by encoding/json without help, so coerce.
	switch vv := v.(type) {
	case []string:
		if vv == nil {
			return "[]", nil
		}
	case []VariationAxis:
		if vv == nil {
			return "[]", nil
		}
	case []CodepointRange:
		if vv == nil {
			return "[]", nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalJSON deserializes "[]" into an empty (non-nil) slice
// equivalent. Propagates other errors. The destination must be a
// pointer to a slice.
func unmarshalJSON(data string, dst interface{}) error {
	if strings.TrimSpace(data) == "" {
		data = "[]"
	}
	if err := json.Unmarshal([]byte(data), dst); err != nil {
		return err
	}
	return nil
}

// SearchFonts runs an FTS5 query against fonts_fts and returns matching
// rows. The query is sanitised so user input cannot escape into FTS5
// operators — this is a "find me anything mentioning these words"
// search, not an advanced query language.
func (s *Store) SearchFonts(ctx context.Context, query string, limit, offset int) ([]Font, error) {
	matchExpr := buildFTSMatchExpression(query)
	if matchExpr == "" {
		return nil, nil
	}

	q := fontSelectColumns() + `
WHERE id IN (
    SELECT rowid FROM fonts_fts WHERE fonts_fts MATCH ?
)
ORDER BY family, subfamily, id`
	args := []interface{}{matchExpr}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			q += " OFFSET ?"
			args = append(args, offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search fonts: %w", err)
	}
	defer rows.Close()

	var out []Font
	for rows.Next() {
		got, err := scanFont(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, rows.Err()
}

// buildFTSMatchExpression returns an FTS5 MATCH expression that finds
// rows containing every token in query. Each token is whitelisted to
// alphanumeric+dash characters then quoted, so user input cannot
// invoke FTS5 operators or syntax. Empty input returns an empty
// string (caller treats this as "no hits").
func buildFTSMatchExpression(query string) string {
	var tokens []string
	var current strings.Builder
	for _, r := range query {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-':
			current.WriteRune(r)
		case r >= 0x80:
			current.WriteRune(r)
		default:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	if len(tokens) == 0 {
		return ""
	}
	for i, tok := range tokens {
		tokens[i] = `"` + tok + `"`
	}
	return strings.Join(tokens, " ")
}
