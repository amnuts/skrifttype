package library

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/amnuts/skrifttype/internal/store"
)

// ScanReport summarises one Scan call. Counts:
//   - Scanned: rows newly inserted or updated (parse + upsert ran).
//   - Skipped: rows where (path, size, mtime) matched and parsing
//     was avoided. The store row's last_seen_at is NOT updated for
//     skipped rows in v1 — Plan 4 / Plan 5 should not rely on it as
//     a freshness signal until the missing-detection logic is
//     reviewed for that consequence.
//   - Errored: parse failures. The font is still recorded in the
//     store with ParseError set so the UI can surface it.
//   - Missing: rows previously available in this watched root that
//     did NOT show up in the walk; they are now marked
//     available=false.
type ScanReport struct {
	Scanned int
	Skipped int
	Errored int
	Missing int
}

// Scanner walks a single watched root, parses fonts via the injected
// Parser, and reconciles its fonts against the store.
//
// The scanner does no concurrency in v1 — files are walked, parsed,
// and upserted sequentially. Plan 5 may add a worker pool here if
// scan latency on large libraries becomes user-visible.
type Scanner struct {
	store  *store.Store
	parser Parser
}

// NewScanner constructs a Scanner. Pass NewSfntParser() in production
// or a test fake (any Parser) in tests.
func NewScanner(s *store.Store, p Parser) *Scanner {
	return &Scanner{store: s, parser: p}
}

// fontExtensions is the whitelist of file types the parser handles:
// plain sfnt (.ttf/.otf), collections (.ttc/.otc, one row per face) and
// WOFF/WOFF2 web fonts. Matching is case-insensitive.
var fontExtensions = map[string]struct{}{
	".ttf":   {},
	".otf":   {},
	".ttc":   {},
	".otc":   {},
	".woff":  {},
	".woff2": {},
}

func isFontPath(path string) bool {
	_, ok := fontExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

// ScanOptions tweaks a scan.
type ScanOptions struct {
	// Force re-parses every file even when (path, size, mtime) match the
	// catalogue. Used after ParserVersion changes.
	Force bool
}

// Scan walks root.Path (recursive iff root.Recursive), upserts each
// font file, and marks any previously-known fonts in this root that
// didn't show up as available=false.
//
// Path canonicalisation: every absolute path the scanner produces
// goes through filepath.Abs before reaching the store, so the store
// can match by string equality. This is the canonical form
// downstream packages MUST use when comparing paths.
func (sc *Scanner) Scan(ctx context.Context, root store.WatchedRoot) (ScanReport, error) {
	return sc.ScanWith(ctx, root, ScanOptions{})
}

// ScanWith is Scan with options.
func (sc *Scanner) ScanWith(ctx context.Context, root store.WatchedRoot, opts ScanOptions) (ScanReport, error) {
	var report ScanReport

	known, err := sc.collectKnownFontIDs(ctx, func(f store.Font) bool { return f.WatchedRootID == root.ID })
	if err != nil {
		return report, fmt.Errorf("collect known: %w", err)
	}
	seen := make(map[int64]struct{}, len(known))

	rootAbs, err := filepath.Abs(root.Path)
	if err != nil {
		return report, fmt.Errorf("abs root path: %w", err)
	}

	walkErr := filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable entry; skip rather than abort the whole walk.
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if !root.Recursive && path != rootAbs {
				return fs.SkipDir
			}
			return nil
		}
		if !isFontPath(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Path is already absolute (rootAbs is absolute and WalkDir
		// joins relative children onto it), but call Abs again
		// defensively — its idempotency is documented and the cost
		// is negligible.
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		sc.ingest(ctx, abs, info, store.FontSourceLibrary, root.ID, opts, seen, &report)
		return nil
	})
	if walkErr != nil {
		return report, fmt.Errorf("walk: %w", walkErr)
	}

	return report, sc.markMissing(ctx, known, seen, &report)
}

// SystemFontFile is one OS-installed font file to catalogue read-only.
type SystemFontFile struct {
	Path   string
	Source store.FontSource // store.FontSourceSystemHKLM or store.FontSourceSystemHKCU
}

// SyncSystemFonts catalogues OS-installed fonts so the library can show
// and preview them. They are never activatable (spec §5 invariant 4).
// Previously-known system fonts absent from files are marked missing.
// Files with an unsupported extension (.fon, ...) are skipped.
func (sc *Scanner) SyncSystemFonts(ctx context.Context, files []SystemFontFile, opts ScanOptions) (ScanReport, error) {
	var report ScanReport
	known, err := sc.collectKnownFontIDs(ctx, func(f store.Font) bool {
		return f.Source == store.FontSourceSystemHKLM || f.Source == store.FontSourceSystemHKCU
	})
	if err != nil {
		return report, fmt.Errorf("collect known: %w", err)
	}
	seen := make(map[int64]struct{}, len(known))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !isFontPath(file.Path) {
			continue
		}
		info, err := os.Stat(file.Path)
		if err != nil || info.IsDir() {
			continue
		}
		sc.ingest(ctx, file.Path, info, file.Source, 0, opts, seen, &report)
	}
	return report, sc.markMissing(ctx, known, seen, &report)
}

// ingest parses one file (unless unchanged) and upserts its faces,
// recording every resulting row ID in seen.
func (sc *Scanner) ingest(ctx context.Context, abs string, info fs.FileInfo, source store.FontSource, rootID int64, opts ScanOptions, seen map[int64]struct{}, report *ScanReport) {
	// Skip when the path has at least one available row and every
	// available row is unchanged. Unavailable rows (e.g. a face that
	// vanished from a rewritten collection) are ignored so they do not
	// force a re-parse on every scan; a path whose rows are all
	// unavailable is re-parsed so a returning file is restored.
	if existing, err := sc.store.ListFontsByPath(ctx, abs); err == nil && !opts.Force {
		var live []store.Font
		unchanged := true
		for _, f := range existing {
			if !f.Available {
				continue
			}
			live = append(live, f)
			if f.FileSize != info.Size() || f.FileMtime != info.ModTime().Unix() || f.Source != source {
				unchanged = false
				break
			}
		}
		if unchanged && len(live) > 0 {
			for _, f := range live {
				seen[f.ID] = struct{}{}
			}
			report.Skipped += len(live)
			return
		}
	}

	records, parseErr := sc.parser.Parse(abs)
	if parseErr != nil {
		// Best-effort: record a row with ParseError so the UI can
		// surface "this font couldn't be parsed" without losing
		// the file.
		records = []FontRecord{{
			Family:     filenameStem(abs),
			Format:     formatFromExt(abs),
			ParseError: parseErr.Error(),
		}}
		report.Errored++
	}

	for _, rec := range records {
		font := store.Font{
			Path:              abs,
			FileSize:          info.Size(),
			FileMtime:         info.ModTime().Unix(),
			Family:            rec.Family,
			Subfamily:         rec.Subfamily,
			FullName:          rec.FullName,
			PostscriptName:    rec.PostscriptName,
			Format:            rec.Format,
			FaceIndex:         rec.FaceIndex,
			Designer:          rec.Designer,
			Foundry:           rec.Foundry,
			LicenseText:       rec.LicenseText,
			VersionString:     rec.VersionString,
			ParseError:        rec.ParseError,
			Classification:    rec.Classification,
			WeightClass:       rec.WeightClass,
			WidthClass:        rec.WidthClass,
			Italic:            rec.Italic,
			IsVariable:        rec.IsVariable,
			GsubFeatures:      rec.GsubFeatures,
			GposFeatures:      rec.GposFeatures,
			UnicodeCodepoints: rec.Codepoints,
			Source:            source,
			WatchedRootID:     rootID,
			Available:         true,
		}
		id, err := sc.store.UpsertFont(ctx, font)
		if err != nil {
			// Per-row failure: count as errored, continue with
			// other faces / files.
			report.Errored++
			continue
		}
		seen[id] = struct{}{}
		if parseErr == nil {
			report.Scanned++
		}
	}
}

// markMissing flags every known-but-unseen font as unavailable.
func (sc *Scanner) markMissing(ctx context.Context, known, seen map[int64]struct{}, report *ScanReport) error {
	for id := range known {
		if _, ok := seen[id]; ok {
			continue
		}
		if err := sc.store.MarkFontUnavailable(ctx, id); err != nil {
			return fmt.Errorf("mark unavailable: %w", err)
		}
		report.Missing++
	}
	return nil
}

// collectKnownFontIDs returns the set of available font IDs matching
// keep. The scanner uses it as the starting point for the
// missing-detection sweep at the end of a scan.
//
// Implementation: ListFonts then filter in-Go. v1's font count is
// well below the threshold where a SQL-side filter would matter; if
// profiling later changes that, add ListFontIDsByWatchedRoot to the
// store.
func (sc *Scanner) collectKnownFontIDs(ctx context.Context, keep func(store.Font) bool) (map[int64]struct{}, error) {
	all, err := sc.store.ListFonts(ctx, store.FontFilter{AvailableOnly: true})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]struct{})
	for _, f := range all {
		if keep(f) {
			out[f.ID] = struct{}{}
		}
	}
	return out, nil
}

// filenameStem returns the filename without its extension. Used as a
// fallback Family when the parser fails entirely.
func filenameStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
