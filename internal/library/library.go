package library

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/amnuts/skrifttype/internal/store"
)

// Library is the top-level coordinator for directory scanning. Wails
// services and main.go consume this; it composes a Scanner with the
// SQL store and a Parser (defaulting to the production sfnt parser).
type Library struct {
	store   *store.Store
	scanner *Scanner

	// mu serialises scans: the scanner's missing-detection sweep is
	// only correct if no other scan is upserting concurrently.
	mu sync.Mutex
}

// New constructs a Library wired with the production sfnt parser.
func New(s *store.Store) *Library {
	return NewLibraryWithParser(s, NewSfntParser())
}

// NewLibraryWithParser is for tests that want to inject a fake parser
// or for future plans that use a different metadata source. Production
// code should use New.
func NewLibraryWithParser(s *store.Store, p Parser) *Library {
	return &Library{
		store:   s,
		scanner: NewScanner(s, p),
	}
}

// ScanAll iterates every enabled watched root and runs Scan on each.
// Returns the aggregated report.
//
// On error, ScanAll returns the aggregate report accumulated so far
// plus the wrapped underlying error. ctx cancellation is honoured
// between roots and within each root's walk.
func (l *Library) ScanAll(ctx context.Context) (ScanReport, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.scanAllLocked(ctx, ScanOptions{})
}

func (l *Library) scanAllLocked(ctx context.Context, opts ScanOptions) (ScanReport, error) {
	var total ScanReport
	roots, err := l.store.ListEnabledWatchedRoots(ctx)
	if err != nil {
		return total, fmt.Errorf("list enabled roots: %w", err)
	}
	for _, r := range roots {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		rep, err := l.scanner.ScanWith(ctx, r, opts)
		total.add(rep)
		if err != nil {
			return total, fmt.Errorf("scan root %d (%s): %w", r.ID, r.Path, err)
		}
	}
	return total, nil
}

// parserVersionKey is the preferences key recording which ParserVersion
// produced the current catalogue.
const parserVersionKey = "library.parserVersion"

// Rescan scans every enabled watched root and syncs the given system
// font files. If the catalogue was built by an older ParserVersion,
// every file is re-parsed once and the new version recorded.
func (l *Library) Rescan(ctx context.Context, system []SystemFontFile) (ScanReport, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	opts := ScanOptions{}
	if v, ok, err := l.store.GetPreference(ctx, parserVersionKey); err != nil {
		return ScanReport{}, fmt.Errorf("read parser version: %w", err)
	} else if n, _ := strconv.Atoi(v); !ok || n < ParserVersion {
		opts.Force = true
	}

	total, err := l.scanAllLocked(ctx, opts)
	if err != nil {
		return total, err
	}
	if system != nil {
		rep, err := l.scanner.SyncSystemFonts(ctx, system, opts)
		total.add(rep)
		if err != nil {
			return total, fmt.Errorf("sync system fonts: %w", err)
		}
	}
	if opts.Force {
		if err := l.store.SetPreference(ctx, parserVersionKey, strconv.Itoa(ParserVersion)); err != nil {
			return total, fmt.Errorf("record parser version: %w", err)
		}
	}
	return total, nil
}

// ScanRoot scans a single watched root.
func (l *Library) ScanRoot(ctx context.Context, root store.WatchedRoot) (ScanReport, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.scanner.Scan(ctx, root)
}

func (r *ScanReport) add(o ScanReport) {
	r.Scanned += o.Scanned
	r.Skipped += o.Skipped
	r.Errored += o.Errored
	r.Missing += o.Missing
}
