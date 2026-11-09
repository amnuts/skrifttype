package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
	"golang.org/x/image/font/sfnt"
)

type nopGDI struct {
	mu     sync.Mutex
	loaded map[string]int
}

func (g *nopGDI) AddFontResource(p string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.loaded[p]++
	return 1, nil
}
func (g *nopGDI) RemoveFontResource(p string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.loaded[p]--
	return nil
}
func (g *nopGDI) BroadcastFontChange() error { return nil }

type events struct {
	mu  sync.Mutex
	got []string
}

func (e *events) emit(name string, _ any) {
	e.mu.Lock()
	e.got = append(e.got, name)
	e.mu.Unlock()
}

type env struct {
	t        *testing.T
	ctx      context.Context
	st       *store.Store
	gdi      *nopGDI
	cat      *Catalog
	lib      *LibraryService
	act      *ActivationService
	col      *CollectionService
	settings *SettingsService
	events   *events
}

// newEnv wires the real services over an in-memory store and a folder
// containing copies of the Inter fixture. Two copies share the family
// "Inter" (different paths), so they group into one family.
func newEnv(t *testing.T, names ...string) (*env, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "library", "testdata", "Inter-Medium.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	e := &env{t: t, ctx: ctx, st: st, gdi: &nopGDI{loaded: map[string]int{}}, events: &events{}}
	e.cat = NewCatalog(st)
	mgr := activation.New(st, e.gdi, nil)
	e.lib = NewLibraryService(ctx, st, library.New(st), e.cat, mgr, nil, e.events.emit, nil)
	e.act = NewActivationService(st, e.cat, mgr, nil)
	e.col = NewCollectionService(st, e.cat, mgr)
	e.settings = NewSettingsService(st, nil, nil, e.lib.Rescan)
	return e, dir
}

func (e *env) addRootAndScan(dir string) WatchedRootDTO {
	e.t.Helper()
	r, err := e.settings.AddWatchedRoot(dir, true)
	if err != nil {
		e.t.Fatal(err)
	}
	e.lib.wait()
	return r
}

func (e *env) onlyFamily() FontDTO {
	e.t.Helper()
	fonts, err := e.lib.ListFonts(FontQueryDTO{Scope: "all"})
	if err != nil {
		e.t.Fatal(err)
	}
	if len(fonts) != 1 {
		e.t.Fatalf("got %d families, want 1: %+v", len(fonts), fonts)
	}
	return fonts[0]
}

func TestCatalog_GroupsFacesIntoFamilies(t *testing.T) {
	e, dir := newEnv(t, "a.ttf", "b.ttf")
	e.addRootAndScan(dir)

	f := e.onlyFamily()
	if f.StyleCount != 2 || len(f.Styles) != 2 {
		t.Fatalf("styles = %+v", f.Styles)
	}
	minID := min(f.Styles[0].ID, f.Styles[1].ID)
	if f.ID != minID {
		t.Errorf("family ID %d, want lowest face ID %d", f.ID, minID)
	}
	if f.CSSFamily != "skrift-"+itoa(f.ID) {
		t.Errorf("CSSFamily = %q", f.CSSFamily)
	}
	if f.Styles[0].URL != "/fontfile/"+itoa(f.Styles[0].ID) || f.Styles[0].Weight != 500 {
		t.Errorf("style = %+v", f.Styles[0])
	}
	if f.Source != store.FontSourceLibrary || !f.Available {
		t.Errorf("source/available = %q/%v", f.Source, f.Available)
	}
	if !slices.Contains(f.GposFeatures, "kern") {
		t.Errorf("GposFeatures = %v", f.GposFeatures)
	}
	if len(f.UnicodeRanges) != 0 {
		t.Error("list results should not carry glyph ranges")
	}
	ranges, err := e.lib.GetGlyphRanges(f.ID)
	if err != nil || len(ranges) == 0 {
		t.Fatalf("GetGlyphRanges = %v, %v", ranges, err)
	}

	got, err := e.lib.GetFont(f.ID)
	if err != nil || got == nil || got.Family != f.Family {
		t.Fatalf("GetFont = %+v, %v", got, err)
	}
	if missing, _ := e.lib.GetFont(999999); missing != nil {
		t.Error("GetFont of unknown ID should be nil")
	}
}

func TestLibraryService_QueryFilters(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	e.addRootAndScan(dir)
	f := e.onlyFamily()

	cases := []struct {
		name string
		q    FontQueryDTO
		want int
	}{
		{"search hit", FontQueryDTO{Search: "INTER"}, 1},
		{"search miss", FontQueryDTO{Search: "garamond"}, 0},
		{"class miss", FontQueryDTO{Classifications: []string{"script"}}, 0},
		{"active none", FontQueryDTO{Scope: "active"}, 0},
	}
	if f.Classification != nil {
		cases = append(cases, struct {
			name string
			q    FontQueryDTO
			want int
		}{"class hit", FontQueryDTO{Classifications: []string{*f.Classification}}, 1})
	}
	for _, c := range cases {
		got, err := e.lib.ListFonts(c.q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d, want %d", c.name, len(got), c.want)
		}
	}

	if _, err := e.act.ActivateManual(f.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.lib.ListFonts(FontQueryDTO{Scope: "active"}); len(got) != 1 {
		t.Errorf("active scope after activation: %d", len(got))
	}
}

func TestActivationService_FamilyLevelToggle(t *testing.T) {
	e, dir := newEnv(t, "a.ttf", "b.ttf")
	e.addRootAndScan(dir)
	f := e.onlyFamily()

	d, err := e.act.ActivateManual(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Loaded, []int64{f.ID}) {
		t.Errorf("Loaded = %v, want [%d]", d.Loaded, f.ID)
	}
	loaded := 0
	for _, n := range e.gdi.loaded {
		loaded += n
	}
	if loaded != 2 {
		t.Errorf("GDI loaded %d files, want both faces", loaded)
	}
	set, err := e.act.GetActiveSet()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(set[f.ID], []string{"manual"}) {
		t.Errorf("active set = %v", set)
	}

	if _, err := e.act.DeactivateManual(f.ID); err != nil {
		t.Fatal(err)
	}
	if set, _ := e.act.GetActiveSet(); len(set) != 0 {
		t.Errorf("active set after deactivate = %v", set)
	}
}

func TestCollectionService_ActiveCollectionFollowsMembership(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	e.addRootAndScan(dir)
	f := e.onlyFamily()

	c, err := e.col.CreateCollection("  Work ", "#12b3a6", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Work" || c.Color != "#12b3a6" || len(c.FontIDs) != 0 {
		t.Fatalf("created = %+v", c)
	}
	if _, err := e.act.ActivateCollection(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.col.AddToCollection(c.ID, []int64{f.ID}); err != nil {
		t.Fatal(err)
	}
	set, _ := e.act.GetActiveSet()
	if !slices.Equal(set[f.ID], []string{"collection:" + itoa(c.ID)}) {
		t.Errorf("active set = %v", set)
	}
	cols, _ := e.col.ListCollections()
	if len(cols) != 1 || !cols[0].Active || !slices.Equal(cols[0].FontIDs, []int64{f.ID}) {
		t.Errorf("collections = %+v", cols)
	}

	if err := e.col.RemoveFromCollection(c.ID, []int64{f.ID}); err != nil {
		t.Fatal(err)
	}
	if set, _ := e.act.GetActiveSet(); len(set) != 0 {
		t.Errorf("removing from active collection left %v", set)
	}

	if err := e.col.AddToCollection(c.ID, []int64{f.ID}); err != nil {
		t.Fatal(err)
	}
	if err := e.col.DeleteCollection(c.ID); err != nil {
		t.Fatal(err)
	}
	if set, _ := e.act.GetActiveSet(); len(set) != 0 {
		t.Errorf("deleting collection left %v", set)
	}
	if _, err := e.col.CreateCollection("   ", "", nil); err == nil {
		t.Error("blank name accepted")
	}
}

func TestSettings_RootChangesRescanAndCleanUp(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	r := e.addRootAndScan(dir)
	f := e.onlyFamily()
	if !slices.Contains(e.events.got, EventLibraryChanged) {
		t.Errorf("events = %v, want %s", e.events.got, EventLibraryChanged)
	}
	if _, err := e.act.ActivateManual(f.ID); err != nil {
		t.Fatal(err)
	}

	// Disabling hides the family and unloads it, but keeps intent.
	if err := e.settings.SetWatchedRootEnabled(r.ID, false); err != nil {
		t.Fatal(err)
	}
	e.lib.wait()
	if got, _ := e.lib.ListFonts(FontQueryDTO{}); len(got) != 0 {
		t.Errorf("disabled root still listed: %d", len(got))
	}
	for p, n := range e.gdi.loaded {
		if n != 0 {
			t.Errorf("%s still loaded after disabling its folder", p)
		}
	}

	// Removing deletes the fonts outright.
	if err := e.settings.RemoveWatchedRoot(r.ID); err != nil {
		t.Fatal(err)
	}
	e.lib.wait()
	all, _ := e.st.ListFonts(e.ctx, store.FontFilter{})
	if len(all) != 0 {
		t.Errorf("fonts left after removing root: %d", len(all))
	}
}

func TestFontFileMiddleware(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	e.addRootAndScan(dir)
	f := e.onlyFamily()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := FontFileMiddleware(e.st)(next)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := get(f.Styles[0].URL)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/ttf" {
		t.Fatalf("GET %s = %d %q", f.Styles[0].URL, rec.Code, rec.Header().Get("Content-Type"))
	}
	body, _ := io.ReadAll(rec.Body)
	want, _ := os.ReadFile(filepath.Join(dir, "a.ttf"))
	if len(body) != len(want) {
		t.Errorf("served %d bytes, want %d", len(body), len(want))
	}
	for _, bad := range []string{"/fontfile/999999", "/fontfile/abc", "/fontfile/-1", "/fontfile/..%2f..%2fwin.ini"} {
		if got := get(bad).Code; got != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", bad, got)
		}
	}
	if got := get("/index.html").Code; got != http.StatusTeapot {
		t.Errorf("non-font path not passed through: %d", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ttcOf wraps single-face fonts in a ttcf collection, shifting each
// face's table offsets by its position in the file.
func ttcOf(fonts ...[]byte) []byte {
	out := make([]byte, 12+4*len(fonts))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(fonts)))
	for i, f := range fonts {
		base := len(out)
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(base))
		c := append([]byte(nil), f...)
		n := int(binary.BigEndian.Uint16(c[4:6]))
		for j := 0; j < n; j++ {
			p := 12 + j*16 + 8
			binary.BigEndian.PutUint32(c[p:], binary.BigEndian.Uint32(c[p:])+uint32(base))
		}
		out = append(out, c...)
	}
	return out
}

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "library", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sfntFamily(t *testing.T, data []byte) string {
	t.Helper()
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	var buf sfnt.Buffer
	name, err := f.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestFontFileServesStandaloneFaceForTTC(t *testing.T) {
	e, dir := newEnv(t)
	ttf := fixtureBytes(t, "Inter-Medium.ttf")
	patched := patchFamilyName(t, ttf)
	origName, patchedName := sfntFamily(t, ttf), sfntFamily(t, patched)
	if origName == patchedName {
		t.Fatal("patch did not change the family name")
	}
	if err := os.WriteFile(filepath.Join(dir, "pair.ttc"), ttcOf(ttf, patched), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	fonts, err := e.lib.ListFonts(FontQueryDTO{Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	var styles []FontStyleDTO
	for _, fam := range fonts {
		styles = append(styles, fam.Styles...)
	}
	if len(styles) != 2 {
		t.Fatalf("styles = %+v", styles)
	}
	h := FontFileMiddleware(e.st)(http.NotFoundHandler())

	do := func(method, url string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// Look each style up by the store row's face index, not list order.
	urlOfFace := func(idx int) string {
		for _, st := range styles {
			row, err := e.st.GetFontByID(e.ctx, st.ID)
			if err != nil {
				t.Fatal(err)
			}
			if row.FaceIndex == idx {
				return st.URL
			}
		}
		t.Fatalf("no style with face index %d", idx)
		return ""
	}

	rec0 := do(http.MethodGet, urlOfFace(0), nil)
	rec1 := do(http.MethodGet, urlOfFace(1), nil)
	for _, rec := range []*httptest.ResponseRecorder{rec0, rec1} {
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/ttf" {
			t.Fatalf("GET = %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		if string(rec.Body.Bytes()[:4]) == "ttcf" {
			t.Fatal("served the whole collection, want a standalone face")
		}
	}
	body := rec1.Body.Bytes()
	if got := sfntFamily(t, rec0.Body.Bytes()); got != origName {
		t.Errorf("face 0 family %q, want %q", got, origName)
	}
	if got := sfntFamily(t, body); got != patchedName {
		t.Errorf("face 1 family %q, want %q", got, patchedName)
	}
	if bytes.Equal(rec0.Body.Bytes(), body) {
		t.Error("faces 0 and 1 served identical bytes")
	}

	url1 := urlOfFace(1)
	rec := do(http.MethodGet, url1, map[string]string{"Range": "bytes=0-3"})
	if rec.Code != http.StatusPartialContent || !bytes.Equal(rec.Body.Bytes(), body[:4]) {
		t.Errorf("Range = %d %x", rec.Code, rec.Body.Bytes())
	}

	rec = do(http.MethodHead, url1, nil)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 ||
		rec.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Errorf("HEAD = %d len=%d CL=%q", rec.Code, rec.Body.Len(), rec.Header().Get("Content-Length"))
	}

	// A second GET is served from the cache and is identical.
	if rec = do(http.MethodGet, url1, nil); !bytes.Equal(rec.Body.Bytes(), body) {
		t.Error("cached response differs")
	}
}

// patchFamilyName returns a copy of an sfnt with the last byte of every
// family-name (name ID 1) string changed in place, so all offsets stay valid.
func patchFamilyName(t *testing.T, ttf []byte) []byte {
	t.Helper()
	out := append([]byte(nil), ttf...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) != "name" {
			continue
		}
		tab := int(binary.BigEndian.Uint32(out[rec+8:]))
		count := int(binary.BigEndian.Uint16(out[tab+2:]))
		strOff := tab + int(binary.BigEndian.Uint16(out[tab+4:]))
		for j := 0; j < count; j++ {
			r := tab + 6 + j*12
			if binary.BigEndian.Uint16(out[r+6:]) != 1 { // family
				continue
			}
			length := int(binary.BigEndian.Uint16(out[r+8:]))
			off := strOff + int(binary.BigEndian.Uint16(out[r+10:]))
			out[off+length-1] ^= 0x20 // flip a letter's case
		}
		return out
	}
	t.Fatal("no name table")
	return nil
}

func TestFontFileServesWOFFRaw(t *testing.T) {
	e, dir := newEnv(t)
	raw := fixtureBytes(t, "Inter-Medium.woff")
	if err := os.WriteFile(filepath.Join(dir, "a.woff"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	f := e.onlyFamily()
	h := FontFileMiddleware(e.st)(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, f.Styles[0].URL, nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/woff" {
		t.Fatalf("GET = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Error("woff bytes were modified")
	}
}

func TestFontContentTypeOTC(t *testing.T) {
	for in, want := range map[string]string{"otc": "font/otf", "otf": "font/otf", "ttc": "font/ttf", "woff2": "font/woff2"} {
		if got := fontContentType(in); got != want {
			t.Errorf("fontContentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStyleActivatableFlag(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	if err := os.WriteFile(filepath.Join(dir, "w.woff"), fixtureBytes(t, "Inter-Medium.woff"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	// A system font row, inserted directly.
	if _, err := e.st.UpsertFont(e.ctx, store.Font{
		Path: `C:\Windows\Fonts\sys.ttf`, FileSize: 1, FileMtime: 1, Family: "Sys",
		Subfamily: "Regular", FullName: "Sys", Format: "ttf",
		Source: store.FontSourceSystemHKLM, Available: true,
	}); err != nil {
		t.Fatal(err)
	}
	e.cat.Invalidate()
	var fonts []FontDTO
	for _, scope := range []string{"all", "system"} {
		part, err := e.lib.ListFonts(FontQueryDTO{Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		fonts = append(fonts, part...)
	}
	got := map[string]bool{}
	for _, fam := range fonts {
		for _, s := range fam.Styles {
			got[filepath.Base(s.Path)] = s.Activatable
		}
	}
	want := map[string]bool{"a.ttf": true, "w.woff": false, "sys.ttf": false}
	for k, v := range want {
		if gv, ok := got[k]; !ok || gv != v {
			t.Errorf("%s activatable = %v (present %v), want %v", k, gv, ok, v)
		}
	}
}

func TestFaceLRUEvictsByByteBudget(t *testing.T) {
	c := newFaceLRU(10, 8)
	builds := 0
	get := func(id int64, n int) {
		_, _ = c.get(faceKey{id: id}, func() ([]byte, error) { builds++; return make([]byte, n), nil })
	}
	get(1, 4)
	get(2, 4)
	get(1, 4) // hit; 2 is now oldest
	get(3, 4) // 12 > 10: evicts 2
	get(1, 4) // still cached
	get(2, 4) // rebuilt, evicts 3
	if builds != 4 {
		t.Errorf("builds = %d, want 4", builds)
	}
	if c.used > 10 {
		t.Errorf("used = %d, over budget", c.used)
	}
}

func TestFaceLRUSkipsOversizeFace(t *testing.T) {
	c := newFaceLRU(100, 8)
	builds := 0
	for i := 0; i < 2; i++ {
		data, err := c.get(faceKey{id: 1}, func() ([]byte, error) { builds++; return make([]byte, 9), nil })
		if err != nil || len(data) != 9 {
			t.Fatalf("get = %d bytes, %v", len(data), err)
		}
	}
	if builds != 2 || c.used != 0 {
		t.Errorf("builds = %d used = %d, want oversize face served uncached", builds, c.used)
	}
}

func TestFaceLRUCollapsesConcurrentMisses(t *testing.T) {
	c := newFaceLRU(100, 50)
	var builds atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.get(faceKey{id: 1}, func() ([]byte, error) {
				builds.Add(1)
				<-release
				return []byte{1, 2, 3}, nil
			})
		}()
	}
	time.Sleep(50 * time.Millisecond) // let the others queue behind the build
	close(release)
	wg.Wait()
	if n := builds.Load(); n != 1 {
		t.Errorf("builds = %d, want 1", n)
	}
}

func TestInstallService_SkipsWebFonts(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	if err := os.WriteFile(filepath.Join(dir, "w.woff"), fixtureBytes(t, "Inter-Medium.woff"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	fam := e.onlyFamily()

	h := &fakeHelper{fontsDir: t.TempDir()}
	svc := NewInstallService(e.st, e.cat, e.act.mgr, nil, h.run, nil)
	res, err := svc.InstallForEveryone(fam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 || len(h.calls[0].Files) != 1 || filepath.Base(h.calls[0].Files[0]) != "a.ttf" {
		t.Errorf("helper calls = %+v, want only a.ttf", h.calls)
	}
	if len(res.Failed) != 1 || res.Failed[0].File != "w.woff" || res.Failed[0].Error != "web fonts cannot be installed on Windows" {
		t.Errorf("Failed = %+v", res.Failed)
	}

	// A web-font-only family never reaches the helper (no UAC prompt).
	e2, dir2 := newEnv(t)
	if err := os.WriteFile(filepath.Join(dir2, "w.woff"), fixtureBytes(t, "Inter-Medium.woff"), 0o644); err != nil {
		t.Fatal(err)
	}
	e2.addRootAndScan(dir2)
	h2 := &fakeHelper{fontsDir: t.TempDir()}
	svc2 := NewInstallService(e2.st, e2.cat, e2.act.mgr, nil, h2.run, nil)
	res, err = svc2.InstallForEveryone(e2.onlyFamily().ID)
	if err != nil || len(res.Failed) != 1 || len(h2.calls) != 0 {
		t.Errorf("res=%+v err=%v helper calls=%+v", res, err, h2.calls)
	}
}
