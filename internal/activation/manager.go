// Package activation is the orchestrator between the activations table
// (the source of truth for which fonts the user wants active) and the
// Win32 GDI font table (what is actually loaded right now).
//
// Every public operation follows the same shape (spec §6):
//
//  1. Commit the change of intent to the store.
//  2. Reconcile: diff the set of paths that *should* be loaded against
//     the set this process has loaded, and issue Add/Remove calls.
//  3. Broadcast WM_FONTCHANGE at most once.
//
// Reconciling against the full desired set (rather than applying the
// per-operation diff) keeps the manager correct across restarts, files
// shared by several rows, roots being disabled, and failed loads that
// should be retried next time.
package activation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/amnuts/skrifttype/internal/store"
	"github.com/amnuts/skrifttype/internal/winfont"
)

// FontError records one font the GDI layer failed to load.
type FontError struct {
	FontID int64
	Path   string
	Err    string
}

// Diff describes the effect of one operation on the global active set.
// Loaded / Unloaded are font IDs that gained their first / lost their
// last activation source. NoOp lists requested fonts whose state did
// not change. Failed lists fonts whose file GDI refused to load; they
// remain active in the store so the next reconcile retries them.
type Diff struct {
	Loaded   []int64
	Unloaded []int64
	NoOp     []int64
	Failed   []FontError
}

// Manager serialises activation changes. It is safe for concurrent use.
type Manager struct {
	store *store.Store
	gdi   winfont.GDI
	log   *slog.Logger

	mu sync.Mutex
	// loaded is the set of absolute paths this process has added to
	// GDI. A path is added once and removed once, regardless of how
	// many font rows reference it.
	loaded map[string]struct{}
}

// New constructs a Manager. Pass winfont.NewGDI() in production.
func New(s *store.Store, gdi winfont.GDI, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{store: s, gdi: gdi, log: log, loaded: make(map[string]struct{})}
}

// ManualSource is the activation source for single-font toggles.
var ManualSource = store.ActivationSource{Kind: store.ActivationKindManual}

// CollectionSource is the activation source for a collection.
func CollectionSource(id int64) store.ActivationSource {
	return store.ActivationSource{Kind: store.ActivationKindCollection, SourceID: sql.NullInt64{Int64: id, Valid: true}}
}

// ActivateManual adds a manual source for each font.
func (m *Manager) ActivateManual(ctx context.Context, fontIDs ...int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.store.ListFontIDsForSource(ctx, ManualSource)
	if err != nil {
		return Diff{}, err
	}
	eligible, rejected, err := m.eligible(ctx, fontIDs)
	if err != nil {
		return Diff{}, err
	}
	d, err := m.applyLocked(ctx, ManualSource, union(current, eligible), fontIDs)
	d.Failed = append(d.Failed, rejected...)
	return d, err
}

// DeactivateManual removes the manual source from each font. Fonts
// still held by a collection stay loaded.
func (m *Manager) DeactivateManual(ctx context.Context, fontIDs ...int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.store.ListFontIDsForSource(ctx, ManualSource)
	if err != nil {
		return Diff{}, err
	}
	return m.applyLocked(ctx, ManualSource, subtract(current, fontIDs), fontIDs)
}

// Deactivate turns each font off outright: it drops the manual source and
// every collection source, so the font unloads even while an active
// collection contains it. Switching the collection off and on again
// restores it.
func (m *Manager) Deactivate(ctx context.Context, fontIDs ...int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := dedupe(fontIDs)
	sd, err := m.store.ClearActivations(ctx, ids)
	if err != nil {
		return Diff{}, err
	}
	d := Diff{Unloaded: sd.NewlyInactive}
	gone := make(map[int64]struct{}, len(d.Unloaded))
	for _, id := range d.Unloaded {
		gone[id] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := gone[id]; !ok {
			d.NoOp = append(d.NoOp, id)
		}
	}
	failed, err := m.reconcileLocked(ctx)
	d.Failed = append(d.Failed, failed...)
	return d, err
}

// ActivateCollection marks the collection active and activates every
// eligible member.
func (m *Manager) ActivateCollection(ctx context.Context, collectionID int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.SetCollectionActive(ctx, collectionID, true); err != nil {
		return Diff{}, err
	}
	return m.syncCollectionLocked(ctx, collectionID)
}

// DeactivateCollection marks the collection inactive and drops its
// activation source. Members also held manually or by another active
// collection stay loaded.
func (m *Manager) DeactivateCollection(ctx context.Context, collectionID int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.SetCollectionActive(ctx, collectionID, false); err != nil {
		return Diff{}, err
	}
	members, err := m.store.ListFontsInCollection(ctx, collectionID)
	if err != nil {
		return Diff{}, err
	}
	return m.applyLocked(ctx, CollectionSource(collectionID), nil, members)
}

// SyncCollection re-applies a collection's activation source from its
// current membership. Call after membership changes; it is a no-op for
// inactive collections.
func (m *Manager) SyncCollection(ctx context.Context, collectionID int64) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncCollectionLocked(ctx, collectionID)
}

func (m *Manager) syncCollectionLocked(ctx context.Context, collectionID int64) (Diff, error) {
	c, err := m.store.GetCollection(ctx, collectionID)
	if err != nil {
		return Diff{}, fmt.Errorf("get collection %d: %w", collectionID, err)
	}
	members, err := m.store.ListFontsInCollection(ctx, collectionID)
	if err != nil {
		return Diff{}, err
	}
	if !c.Active {
		return m.applyLocked(ctx, CollectionSource(collectionID), nil, members)
	}
	eligible, rejected, err := m.eligible(ctx, members)
	if err != nil {
		return Diff{}, err
	}
	d, err := m.applyLocked(ctx, CollectionSource(collectionID), eligible, members)
	d.Failed = append(d.Failed, rejected...)
	return d, err
}

// Reconcile brings the GDI font table in line with the store without
// changing any intent. Use it on launch (spec "RestoreOnLaunch") and
// after a rescan changes which files are available.
func (m *Manager) Reconcile(ctx context.Context) ([]FontError, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reconcileLocked(ctx)
}

// maxStaleRefs bounds how many times SweepStale unloads one path; a font
// loaded repeatedly by earlier runs needs one removal per load.
const maxStaleRefs = 32

// SweepStale unloads library fonts that Windows still has loaded but that
// nothing wants: leftovers from earlier runs that were killed before they
// could unload, or from older builds. The manager only tracks what it
// loaded itself, so without this they stay in the session (and in other
// apps' font lists) until the user logs off. Returns how many font files
// were unloaded. Slow when there is a lot to do, so run it in the
// background; it yields the lock between files.
func (m *Manager) SweepStale(ctx context.Context) (int, error) {
	paths, err := m.store.ListLibraryPaths(ctx)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	want, err := m.store.ListLoadableActivePaths(ctx)
	m.mu.Unlock()
	if err != nil {
		return 0, err
	}
	wanted := make(map[string]struct{}, len(want))
	for _, p := range want {
		wanted[p] = struct{}{}
	}
	swept := 0
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			break
		}
		if _, ok := wanted[p]; ok {
			continue
		}
		m.mu.Lock()
		if _, ok := m.loaded[p]; !ok {
			removed := false
			for i := 0; i < maxStaleRefs && m.gdi.RemoveFontResource(p) == nil; i++ {
				removed = true
			}
			if removed {
				swept++
			}
		}
		m.mu.Unlock()
	}
	if swept > 0 {
		if err := m.gdi.BroadcastFontChange(); err != nil && !errors.Is(err, winfont.ErrUnsupportedPlatform) {
			return swept, err
		}
	}
	return swept, nil
}

// Rebroadcast re-sends WM_FONTCHANGE without changing anything. Useful
// when an application missed the original broadcast.
func (m *Manager) Rebroadcast() error {
	return m.gdi.BroadcastFontChange()
}

// UnloadAll removes every font this process loaded and broadcasts once.
// Intent in the store is untouched, so the next launch restores them.
//
// Fonts added with AddFontResourceEx and dwFlags=0 are not private to
// the process: Windows keeps them until logoff unless they are removed.
// Call this on shutdown so "quit" really means "deactivate", and so
// the next launch's restore does not stack a second GDI reference.
func (m *Manager) UnloadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.loaded) == 0 {
		return nil
	}
	var errs []error
	for path := range m.loaded {
		if err := m.gdi.RemoveFontResource(path); err != nil && !errors.Is(err, winfont.ErrUnsupportedPlatform) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
		delete(m.loaded, path)
	}
	if err := m.gdi.BroadcastFontChange(); err != nil && !errors.Is(err, winfont.ErrUnsupportedPlatform) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// applyLocked replaces the rows for src with desired, then reconciles.
// requested is the set the caller asked about, used to fill NoOp.
func (m *Manager) applyLocked(ctx context.Context, src store.ActivationSource, desired, requested []int64) (Diff, error) {
	sd, err := m.store.ReplaceActivationSet(ctx, src, desired)
	if err != nil {
		return Diff{}, err
	}
	d := Diff{Loaded: sd.NewlyActive, Unloaded: sd.NewlyInactive}
	changed := make(map[int64]struct{}, len(d.Loaded)+len(d.Unloaded))
	for _, id := range d.Loaded {
		changed[id] = struct{}{}
	}
	for _, id := range d.Unloaded {
		changed[id] = struct{}{}
	}
	for _, id := range dedupe(requested) {
		if _, ok := changed[id]; !ok {
			d.NoOp = append(d.NoOp, id)
		}
	}
	failed, err := m.reconcileLocked(ctx)
	d.Failed = append(d.Failed, failed...)
	return d, err
}

// reconcileLocked diffs desired paths against m.loaded, issues GDI
// calls, and broadcasts once if anything changed.
func (m *Manager) reconcileLocked(ctx context.Context) ([]FontError, error) {
	want, err := m.store.ListLoadableActivePaths(ctx)
	if err != nil {
		return nil, err
	}
	wantPaths := make(map[string][]int64, len(want))
	for id, p := range want {
		wantPaths[p] = append(wantPaths[p], id)
	}

	var failed []FontError
	changed := false

	// Deterministic order keeps tests and logs stable.
	paths := make([]string, 0, len(wantPaths))
	for p := range wantPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if _, ok := m.loaded[p]; ok {
			continue
		}
		if _, err := m.gdi.AddFontResource(p); err != nil {
			if errors.Is(err, winfont.ErrUnsupportedPlatform) {
				continue // non-Windows dev build: intent only
			}
			m.log.Warn("activation: load failed", "path", p, "err", err)
			for _, id := range wantPaths[p] {
				failed = append(failed, FontError{FontID: id, Path: p, Err: err.Error()})
			}
			continue
		}
		m.loaded[p] = struct{}{}
		changed = true
	}

	var stale []string
	for p := range m.loaded {
		if _, ok := wantPaths[p]; !ok {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	for _, p := range stale {
		if err := m.gdi.RemoveFontResource(p); err != nil && !errors.Is(err, winfont.ErrUnsupportedPlatform) {
			// Keep going: Windows may already have dropped it (file
			// deleted); holding the entry would retry forever.
			m.log.Warn("activation: unload failed", "path", p, "err", err)
		}
		delete(m.loaded, p)
		changed = true
	}

	if changed {
		if err := m.gdi.BroadcastFontChange(); err != nil && !errors.Is(err, winfont.ErrUnsupportedPlatform) {
			return failed, fmt.Errorf("broadcast: %w", err)
		}
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i].FontID < failed[j].FontID })
	return failed, nil
}

// eligible splits ids into those the manager may activate (library
// fonts with an available file) and rejections with a reason.
func (m *Manager) eligible(ctx context.Context, ids []int64) ([]int64, []FontError, error) {
	var ok []int64
	var rejected []FontError
	for _, id := range dedupe(ids) {
		f, err := m.store.GetFontByID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			rejected = append(rejected, FontError{FontID: id, Err: "font not found"})
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get font %d: %w", id, err)
		}
		switch {
		case f.Format == "woff" || f.Format == "woff2":
			rejected = append(rejected, FontError{FontID: id, Path: f.Path, Err: "web fonts cannot be activated on Windows"})
		case f.Source != store.FontSourceLibrary:
			rejected = append(rejected, FontError{FontID: id, Path: f.Path, Err: "system fonts cannot be toggled"})
		case !f.Available:
			rejected = append(rejected, FontError{FontID: id, Path: f.Path, Err: "font file is missing"})
		default:
			ok = append(ok, id)
		}
	}
	return ok, rejected, nil
}

func dedupe(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func union(a, b []int64) []int64 { return dedupe(append(append([]int64{}, a...), b...)) }

func subtract(a, b []int64) []int64 {
	drop := make(map[int64]struct{}, len(b))
	for _, id := range b {
		drop[id] = struct{}{}
	}
	var out []int64
	for _, id := range a {
		if _, ok := drop[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}
