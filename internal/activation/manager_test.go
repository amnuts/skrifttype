package activation_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/store"
)

// fakeGDI records calls and tracks a per-path reference count the way
// Windows does.
type fakeGDI struct {
	refs       map[string]int
	adds       []string
	removes    []string
	broadcasts int
	failAdd    map[string]bool
}

func newFakeGDI() *fakeGDI {
	return &fakeGDI{refs: map[string]int{}, failAdd: map[string]bool{}}
}

func (g *fakeGDI) AddFontResource(path string) (int, error) {
	if g.failAdd[path] {
		return 0, errors.New("AddFontResourceExW returned 0")
	}
	g.adds = append(g.adds, path)
	g.refs[path]++
	return 1, nil
}

func (g *fakeGDI) RemoveFontResource(path string) error {
	if g.refs[path] <= 0 {
		return errors.New("RemoveFontResourceExW returned 0")
	}
	g.removes = append(g.removes, path)
	g.refs[path]--
	return nil
}

func (g *fakeGDI) BroadcastFontChange() error { g.broadcasts++; return nil }

func (g *fakeGDI) loadedPaths() []string {
	var out []string
	for p, n := range g.refs {
		if n > 0 {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

type fixture struct {
	ctx context.Context
	st  *store.Store
	gdi *fakeGDI
	m   *activation.Manager
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	gdi := newFakeGDI()
	return &fixture{ctx: ctx, st: st, gdi: gdi, m: activation.New(st, gdi, nil)}
}

func (f *fixture) font(t *testing.T, path string, mutate ...func(*store.Font)) int64 {
	t.Helper()
	font := store.Font{
		Path: path, FileSize: 1, FileMtime: 1,
		Family: "F", Subfamily: "Regular", FullName: "F Regular", Format: "ttf",
		Source: store.FontSourceLibrary, Available: true,
	}
	for _, fn := range mutate {
		fn(&font)
	}
	id, err := f.st.UpsertFont(f.ctx, font)
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	return id
}

func TestActivateManual_LoadsAndBroadcastsOnce(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	b := f.font(t, `D:\f\b.ttf`)

	d, err := f.m.ActivateManual(f.ctx, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Loaded, []int64{a, b}) {
		t.Errorf("Loaded = %v", d.Loaded)
	}
	if got := f.gdi.loadedPaths(); !reflect.DeepEqual(got, []string{`D:\f\a.ttf`, `D:\f\b.ttf`}) {
		t.Errorf("loaded paths = %v", got)
	}
	if f.gdi.broadcasts != 1 {
		t.Errorf("broadcasts = %d, want 1", f.gdi.broadcasts)
	}

	// Re-activating is a no-op: no GDI calls, no broadcast.
	d, err = f.m.ActivateManual(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.NoOp, []int64{a}) || len(d.Loaded) != 0 {
		t.Errorf("second activate diff = %+v", d)
	}
	if len(f.gdi.adds) != 2 || f.gdi.broadcasts != 1 {
		t.Errorf("adds=%d broadcasts=%d after no-op", len(f.gdi.adds), f.gdi.broadcasts)
	}
}

func TestDeactivateManual_KeepsFontHeldByCollection(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	c, err := f.st.CreateCollection(f.ctx, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.AddFontToCollection(f.ctx, c.ID, a); err != nil {
		t.Fatal(err)
	}

	if _, err := f.m.ActivateManual(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.ActivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	d, err := f.m.DeactivateManual(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Unloaded) != 0 {
		t.Errorf("font still held by collection was unloaded: %+v", d)
	}
	if got := f.gdi.loadedPaths(); len(got) != 1 {
		t.Errorf("loaded = %v, want the font still loaded", got)
	}

	d, err = f.m.DeactivateCollection(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Unloaded, []int64{a}) {
		t.Errorf("Unloaded = %v", d.Unloaded)
	}
	if got := f.gdi.loadedPaths(); len(got) != 0 {
		t.Errorf("loaded = %v, want none", got)
	}
}

func TestDeactivate_UnloadsFontHeldByActiveCollection(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	b := f.font(t, `D:\f\b.ttf`)
	c, err := f.st.CreateCollection(f.ctx, "Work")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a, b} {
		if err := f.st.AddFontToCollection(f.ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.m.ActivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	d, err := f.m.Deactivate(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Unloaded, []int64{a}) {
		t.Errorf("Unloaded = %v, want [a]", d.Unloaded)
	}
	if got := f.gdi.loadedPaths(); len(got) != 1 {
		t.Errorf("loaded = %v, want only b", got)
	}

	// Switching the collection off clears everything, with nothing left behind.
	if _, err := f.m.DeactivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 0 {
		t.Errorf("loaded = %v, want none", got)
	}
	// Switching it back on restores the font.
	if _, err := f.m.ActivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 2 {
		t.Errorf("loaded = %v, want both", got)
	}
}

func TestSweepStale_UnloadsOrphansButKeepsActiveFonts(t *testing.T) {
	f := setup(t)
	keep := `D:\f\keep.ttf`
	orphan := `D:\f\orphan.ttf`
	twice := `D:\f\twice.ttf`
	active := f.font(t, keep)
	f.font(t, orphan)
	f.font(t, twice)
	if _, err := f.m.ActivateManual(f.ctx, active); err != nil {
		t.Fatal(err)
	}
	// Loaded by an earlier run the manager knows nothing about.
	for _, p := range []string{orphan, twice, twice} {
		if _, err := f.gdi.AddFontResource(p); err != nil {
			t.Fatal(err)
		}
	}
	n, err := f.m.SweepStale(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("swept = %d, want 2", n)
	}
	if got := f.gdi.loadedPaths(); !reflect.DeepEqual(got, []string{keep}) {
		t.Errorf("loaded = %v, want only the active font", got)
	}
}

func TestActivateCollection_OneBroadcastForManyFonts(t *testing.T) {
	f := setup(t)
	c, _ := f.st.CreateCollection(f.ctx, "Big")
	for i := 0; i < 50; i++ {
		id := f.font(t, `D:\f\`+string(rune('a'+i%26))+string(rune('a'+i/26))+`.ttf`)
		_ = f.st.AddFontToCollection(f.ctx, c.ID, id)
	}
	if _, err := f.m.ActivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.gdi.adds) != 50 || f.gdi.broadcasts != 1 {
		t.Errorf("adds=%d broadcasts=%d, want 50 and 1", len(f.gdi.adds), f.gdi.broadcasts)
	}
	got, _ := f.st.GetCollection(f.ctx, c.ID)
	if !got.Active {
		t.Error("collection not marked active")
	}
}

func TestSyncCollection_FollowsMembershipOnlyWhenActive(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	c, _ := f.st.CreateCollection(f.ctx, "C")

	// Inactive collection: membership changes load nothing.
	_ = f.st.AddFontToCollection(f.ctx, c.ID, a)
	if _, err := f.m.SyncCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.gdi.adds) != 0 {
		t.Fatalf("inactive collection loaded fonts")
	}

	// Empty-then-populated active collection picks up new members.
	if _, err := f.m.ActivateCollection(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	b := f.font(t, `D:\f\b.ttf`)
	_ = f.st.AddFontToCollection(f.ctx, c.ID, b)
	d, err := f.m.SyncCollection(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Loaded, []int64{b}) {
		t.Errorf("Loaded = %v, want [%d]", d.Loaded, b)
	}
}

func TestActivate_RejectsSystemAndMissingFonts(t *testing.T) {
	f := setup(t)
	sys := f.font(t, `C:\Windows\Fonts\arial.ttf`, func(x *store.Font) { x.Source = store.FontSourceSystemHKLM })
	gone := f.font(t, `D:\f\gone.ttf`, func(x *store.Font) { x.Available = false })

	d, err := f.m.ActivateManual(f.ctx, sys, gone)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Failed) != 2 || len(d.Loaded) != 0 {
		t.Errorf("diff = %+v, want both rejected", d)
	}
	if len(f.gdi.adds) != 0 {
		t.Errorf("GDI touched for ineligible fonts: %v", f.gdi.adds)
	}
	if active, _ := f.st.IsActive(f.ctx, sys); active {
		t.Error("system font recorded as active")
	}
}

func TestFailedLoadStaysIntendedAndRetries(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	f.gdi.failAdd[`D:\f\a.ttf`] = true

	d, err := f.m.ActivateManual(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Failed) != 1 || d.Failed[0].FontID != a {
		t.Fatalf("Failed = %+v", d.Failed)
	}
	if active, _ := f.st.IsActive(f.ctx, a); !active {
		t.Error("intent should survive a failed load")
	}

	f.gdi.failAdd[`D:\f\a.ttf`] = false
	failed, err := f.m.Reconcile(f.ctx)
	if err != nil || len(failed) != 0 {
		t.Fatalf("Reconcile = %v, %v", failed, err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 1 {
		t.Errorf("retry did not load: %v", got)
	}
}

func TestReconcile_RestoresOnLaunchAndUnloadAllReleases(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	if _, err := f.st.AddActivationSource(f.ctx, a, activation.ManualSource); err != nil {
		t.Fatal(err)
	}

	// Fresh manager == fresh process: nothing loaded until Reconcile.
	if _, err := f.m.Reconcile(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 1 || f.gdi.broadcasts != 1 {
		t.Fatalf("restore: loaded=%v broadcasts=%d", got, f.gdi.broadcasts)
	}

	if err := f.m.UnloadAll(); err != nil {
		t.Fatal(err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 0 {
		t.Errorf("UnloadAll left %v loaded", got)
	}
	if f.gdi.broadcasts != 2 {
		t.Errorf("broadcasts = %d, want 2", f.gdi.broadcasts)
	}
	if active, _ := f.st.IsActive(f.ctx, a); !active {
		t.Error("UnloadAll must not clear intent")
	}
}

func TestReconcile_DropsFontsThatWentMissing(t *testing.T) {
	f := setup(t)
	a := f.font(t, `D:\f\a.ttf`)
	if _, err := f.m.ActivateManual(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := f.st.MarkFontUnavailable(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Reconcile(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.gdi.loadedPaths(); len(got) != 0 {
		t.Errorf("missing font still loaded: %v", got)
	}
}

func TestActivateWOFFRefused(t *testing.T) {
	f := setup(t)
	woff := f.font(t, `D:\f\w.woff`, func(x *store.Font) { x.Format = "woff" })
	woff2 := f.font(t, `D:\f\w.woff2`, func(x *store.Font) { x.Format = "woff2" })
	ok := f.font(t, `D:\f\ok.ttf`)

	// Manual activation: refused with a per-font error, GDI untouched.
	d, err := f.m.ActivateManual(f.ctx, woff, woff2)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Failed) != 2 || len(d.Loaded) != 0 {
		t.Fatalf("diff = %+v, want both refused", d)
	}
	for _, fe := range d.Failed {
		if fe.Err != "web fonts cannot be activated on Windows" {
			t.Errorf("Err = %q", fe.Err)
		}
	}
	if len(f.gdi.adds) != 0 {
		t.Fatalf("GDI touched for web fonts: %v", f.gdi.adds)
	}
	if active, _ := f.st.IsActive(f.ctx, woff); active {
		t.Error("web font recorded as active")
	}

	// A web font that is a member of an active collection is refused
	// while its siblings still load.
	c, _ := f.st.CreateCollection(f.ctx, "Mixed")
	_ = f.st.AddFontToCollection(f.ctx, c.ID, woff)
	_ = f.st.AddFontToCollection(f.ctx, c.ID, ok)
	d, err = f.m.ActivateCollection(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Failed) != 1 || d.Failed[0].FontID != woff {
		t.Errorf("Failed = %+v", d.Failed)
	}
	if got := f.gdi.loadedPaths(); !reflect.DeepEqual(got, []string{`D:\f\ok.ttf`}) {
		t.Errorf("loaded = %v, want only ok.ttf", got)
	}

	// A stale activation row (written before web fonts were refused, or
	// by a font whose format changed on rescan) must not reach GDI on
	// reconcile either.
	if _, err := f.st.AddActivationSource(f.ctx, woff2, store.ActivationSource{Kind: store.ActivationKindManual}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Reconcile(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.gdi.adds {
		if p == `D:\f\w.woff` || p == `D:\f\w.woff2` {
			t.Errorf("GDI AddFontResource called for web font %s", p)
		}
	}
}
