package api

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
	"github.com/amnuts/skrifttype/internal/winfont"
)

// Event names emitted to the frontend.
const (
	// EventLibraryScanning carries a bool: true when a rescan starts,
	// false when the last queued rescan finishes.
	EventLibraryScanning = "library:scanning"
	// EventLibraryChanged carries a ScanReportDTO after each rescan.
	EventLibraryChanged = "library:changed"
	// EventActivationProgress carries an ActivationProgressDTO while a bulk
	// on/off of many fonts runs.
	EventActivationProgress = "activation:progress"
)

// Emitter sends an event to the frontend. main wires it to
// app.Event.Emit; tests pass a recorder or nil.
type Emitter func(name string, data any)

// FontQueryDTO filters ListFonts. TS's `scope: 'all' | 'active' | number`
// is split into Scope ("all" | "system" | "active" | "collection") +
// CollectionID. "all" is the library fonts (those from watched folders);
// "system" is the fonts installed in the OS.
type FontQueryDTO struct {
	Search          string   `json:"search"`
	Classifications []string `json:"classifications"`
	Scope           string   `json:"scope"`
	CollectionID    int64    `json:"collectionId"`
	Sort            string   `json:"sort"` // "family" (default) | "styles"
}

// ScanReportDTO summarises a rescan.
type ScanReportDTO struct {
	Scanned int    `json:"scanned"`
	Skipped int    `json:"skipped"`
	Errored int    `json:"errored"`
	Missing int    `json:"missing"`
	Error   string `json:"error,omitempty"`
}

// LibraryService exposes the font catalogue and rescanning.
type LibraryService struct {
	store   *store.Store
	lib     *library.Library
	cat     *Catalog
	mgr     *activation.Manager
	sysEnum winfont.SystemFontEnumerator
	emit    Emitter
	log     *slog.Logger

	// ctx bounds background rescans; cancelled on shutdown.
	ctx context.Context

	mu      sync.Mutex
	running bool
	again   bool
	done    chan struct{} // closed when the current rescan loop exits
}

// NewLibraryService wires the service. ctx should be cancelled on
// application shutdown so an in-flight scan stops promptly.
func NewLibraryService(ctx context.Context, st *store.Store, lib *library.Library, cat *Catalog,
	mgr *activation.Manager, sysEnum winfont.SystemFontEnumerator, emit Emitter, log *slog.Logger) *LibraryService {
	if emit == nil {
		emit = func(string, any) {}
	}
	if log == nil {
		log = slog.Default()
	}
	return &LibraryService{ctx: ctx, store: st, lib: lib, cat: cat, mgr: mgr, sysEnum: sysEnum, emit: emit, log: log}
}

// ListFonts returns the families matching q.
func (s *LibraryService) ListFonts(q FontQueryDTO) ([]FontDTO, error) {
	ctx := context.Background()
	idx, err := s.cat.index(ctx)
	if err != nil {
		return nil, err
	}

	var scope map[int64]struct{}
	var source string // restricts to library or system fonts when set
	switch q.Scope {
	case "all":
		source = store.FontSourceLibrary
	case "system":
		source = "system"
	case "active":
		ids, err := s.store.ListActiveFontIDs(ctx)
		if err != nil {
			return nil, err
		}
		scope = toSet(idx.familiesOf(ids))
	case "collection":
		ids, err := s.store.ListFontsInCollection(ctx, q.CollectionID)
		if err != nil {
			return nil, err
		}
		scope = toSet(idx.familiesOf(ids))
	}

	search := strings.ToLower(strings.TrimSpace(q.Search))
	out := make([]FontDTO, 0, len(idx.families))
	for _, fam := range idx.families {
		d := fam.dto
		if source != "" && (d.Source == store.FontSourceLibrary) != (source == store.FontSourceLibrary) {
			continue
		}
		if scope != nil {
			if _, ok := scope[d.ID]; !ok {
				continue
			}
		}
		if len(q.Classifications) > 0 && (d.Classification == nil || !slices.Contains(q.Classifications, *d.Classification)) {
			continue
		}
		if search != "" && !matches(d, search) {
			continue
		}
		out = append(out, d)
	}
	if q.Sort == "styles" {
		slices.SortStableFunc(out, func(a, b FontDTO) int { return cmp.Compare(b.StyleCount, a.StyleCount) })
	}
	return out, nil
}

// ScopeCountsDTO is the number of families in the two library-wide scopes.
type ScopeCountsDTO struct {
	System    int `json:"system"`
	Available int `json:"available"`
	// Activatable is how many Available families can be switched on: those
	// with at least one present, non-web-font face.
	Activatable int `json:"activatable"`
}

// ScopeCounts returns how many families the System and Available scopes hold,
// ignoring any search or classification filter.
func (s *LibraryService) ScopeCounts() (ScopeCountsDTO, error) {
	idx, err := s.cat.index(context.Background())
	if err != nil {
		return ScopeCountsDTO{}, err
	}
	var c ScopeCountsDTO
	for _, fam := range idx.families {
		if fam.dto.Source == store.FontSourceLibrary {
			c.Available++
			if slices.ContainsFunc(fam.dto.Styles, func(st FontStyleDTO) bool { return st.Activatable && st.Available }) {
				c.Activatable++
			}
		} else {
			c.System++
		}
	}
	return c, nil
}

func matches(d FontDTO, q string) bool {
	if strings.Contains(strings.ToLower(d.Family), q) {
		return true
	}
	for _, p := range []*string{d.Designer, d.Foundry} {
		if p != nil && strings.Contains(strings.ToLower(*p), q) {
			return true
		}
	}
	return false
}

// GetFont returns one family, or null if the ID is unknown.
func (s *LibraryService) GetFont(id int64) (*FontDTO, error) {
	idx, err := s.cat.index(context.Background())
	if err != nil {
		return nil, err
	}
	fam, ok := idx.byID[id]
	if !ok {
		return nil, nil
	}
	d := fam.dto
	return &d, nil
}

// GetGlyphRanges returns the family's merged cmap coverage.
func (s *LibraryService) GetGlyphRanges(id int64) ([]RangeDTO, error) {
	idx, err := s.cat.index(context.Background())
	if err != nil {
		return nil, err
	}
	fam, ok := idx.byID[id]
	if !ok {
		return []RangeDTO{}, nil
	}
	out := make([]RangeDTO, 0, len(fam.ranges))
	for _, r := range fam.ranges {
		out = append(out, RangeDTO{Start: r.Lo, End: r.Hi})
	}
	return out, nil
}

// IsScanning reports whether a rescan is in progress.
func (s *LibraryService) IsScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Rescan queues a rescan of every enabled folder and the system fonts
// and returns immediately. Requests made while a scan is running are
// coalesced into one follow-up scan. Progress arrives as
// library:scanning / library:changed events.
func (s *LibraryService) Rescan() {
	s.mu.Lock()
	if s.running {
		s.again = true
		s.mu.Unlock()
		return
	}
	s.running = true
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()

	s.emit(EventLibraryScanning, true)
	go func() {
		defer close(done)
		for {
			s.emit(EventLibraryChanged, s.rescanOnce(s.ctx))
			s.mu.Lock()
			if !s.again || s.ctx.Err() != nil {
				s.running = false
				s.mu.Unlock()
				s.emit(EventLibraryScanning, false)
				return
			}
			s.again = false
			s.mu.Unlock()
		}
	}()
}

// wait blocks until any in-flight rescan loop has finished. For tests
// and orderly shutdown.
func (s *LibraryService) wait() {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (s *LibraryService) rescanOnce(ctx context.Context) ScanReportDTO {
	var system []library.SystemFontFile
	if s.sysEnum != nil {
		fonts, err := s.sysEnum.EnumerateSystemFonts()
		switch {
		case errors.Is(err, winfont.ErrUnsupportedPlatform):
			// Leave any existing system rows alone.
		case err != nil:
			s.log.Warn("library: enumerate system fonts", "err", err)
		default:
			system = make([]library.SystemFontFile, 0, len(fonts))
			for _, f := range fonts {
				system = append(system, library.SystemFontFile{Path: f.FilePath, Source: string(f.Source)})
			}
		}
	}

	rep, err := s.lib.Rescan(ctx, system)
	dto := ScanReportDTO{Scanned: rep.Scanned, Skipped: rep.Skipped, Errored: rep.Errored, Missing: rep.Missing}
	if err != nil {
		s.log.Warn("library: rescan", "err", err)
		dto.Error = err.Error()
	}
	s.cat.Invalidate()
	// Files may have appeared, vanished, or had their folder disabled.
	if _, err := s.mgr.Reconcile(ctx); err != nil {
		s.log.Warn("library: reconcile after rescan", "err", err)
	}
	return dto
}

func toSet(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
