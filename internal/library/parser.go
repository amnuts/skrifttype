// Package library is the directory-scanner and metadata-parser for
// skrifttype. It walks user-configured watched roots, parses
// OpenType metadata from font files, and upserts records into the
// internal/store package.
//
// This package depends only on internal/store (the SQLite layer),
// golang.org/x/image/font/sfnt (a pure-Go OpenType parser) and
// github.com/fsnotify/fsnotify (the live folder watcher). It is
// platform-neutral.
//
// Supported inputs: TTF, OTF, TTC/OTC collections, and WOFF/WOFF2 web
// fonts (see container.go).
package library

import "github.com/amnuts/skrifttype/internal/store"

// ParserVersion identifies what the parser extracts. Bump it whenever
// the parser starts reading new data so existing catalogues re-parse
// files whose (size, mtime) have not changed. Version 5 added TTC/OTC
// and WOFF/WOFF2 parsing with per-face records.
const ParserVersion = 5

// FontRecord is the parsed metadata for one font face. The scanner
// combines it with file-system metadata (path, size, mtime) and
// catalogue metadata (watched_root_id, source) to produce a
// store.Font.
//
// All string fields are best-effort: if a font omits a particular
// name table entry, the field is empty. If parsing fails completely,
// ParseError is non-empty and the other fields hold whatever was
// recoverable (typically just Format and a filename-derived Family).
type FontRecord struct {
	Family         string
	Subfamily      string
	FullName       string
	PostscriptName string
	Designer       string
	Foundry        string
	LicenseText    string
	VersionString  string
	Format         string // "ttf"|"otf"|"ttc"|"otc"|"woff"|"woff2"
	FaceIndex      int    // index within a collection; 0 for single-face files
	ParseError     string // non-empty if parse partially failed
	Classification string // "serif"|"sans"|"mono"|"script"|"display"|""

	WeightClass  int  // OS/2 usWeightClass; 0 if unknown
	WidthClass   int  // OS/2 usWidthClass; 0 if unknown
	Italic       bool // OS/2 fsSelection ITALIC, or an italic/oblique subfamily
	IsVariable   bool // has an fvar table
	GsubFeatures []string
	GposFeatures []string
	Codepoints   []store.CodepointRange // cmap coverage, merged ranges
}

// Parser extracts metadata from a font file at the given absolute
// path. A successful Parse returns at least one FontRecord per file
// — one for single-face TTF/OTF, multiple for TTC/OTC (Plan 3c).
// If the file cannot be parsed at all, Parse returns a non-nil
// error AND zero records; the scanner's responsibility is to record
// a parse_error row in the store via the FontRecord.ParseError field
// rather than dropping the file silently.
//
// Implementations MUST be safe to call concurrently for different
// paths (the scanner is currently sequential, but the contract leaves
// room for a future worker pool).
type Parser interface {
	Parse(path string) ([]FontRecord, error)
}
