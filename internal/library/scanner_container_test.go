package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

func openScanStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func addRoot(t *testing.T, s *store.Store, dir string) store.WatchedRoot {
	t.Helper()
	root, err := s.AddWatchedRoot(context.Background(), dir, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}
	return root
}

func absPath(t *testing.T, dir, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanTTCCreatesRowPerFace(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	ttf := readFixture(t, "Inter-Medium.ttf")
	if err := os.WriteFile(filepath.Join(dir, "pair.ttc"), buildTTC(t, ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	root := addRoot(t, s, dir)
	rep, err := NewScanner(s, NewSfntParser()).Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2", rep.Scanned)
	}
	rows, err := s.ListFontsByPath(ctx, absPath(t, dir, "pair.ttc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].FaceIndex != 0 || rows[1].FaceIndex != 1 {
		t.Fatalf("rows = %+v, want faces 0 and 1", rows)
	}
	for _, r := range rows {
		if r.Format != "ttc" || !r.Available {
			t.Errorf("face %d: format %q available %v", r.FaceIndex, r.Format, r.Available)
		}
	}

	// Unchanged rescan skips the file without re-parsing.
	rep, err = NewScanner(s, NewSfntParser()).Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped != 2 || rep.Scanned != 0 {
		t.Errorf("rescan = %+v, want Skipped 2 Scanned 0", rep)
	}
}

func TestRescanShrunkTTCMarksMissingFace(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	ttf := readFixture(t, "Inter-Medium.ttf")
	path := filepath.Join(dir, "pair.ttc")
	if err := os.WriteFile(path, buildTTC(t, ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	root := addRoot(t, s, dir)
	sc := NewScanner(s, NewSfntParser())
	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Rewrite as a 1-face collection (different size, so it is not skipped).
	if err := os.WriteFile(path, buildTTC(t, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Missing != 1 {
		t.Errorf("Missing = %d, want 1", rep.Missing)
	}
	abs := absPath(t, dir, "pair.ttc")
	f0, err := s.GetFontByPathFace(ctx, abs, 0)
	if err != nil || !f0.Available {
		t.Errorf("face 0: %+v err %v, want available", f0, err)
	}
	f1, err := s.GetFontByPathFace(ctx, abs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f1.Available {
		t.Error("face 1 still available after shrink")
	}
}

func TestScanWOFFCataloguedWithFormat(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	for _, n := range []string{"Inter-Medium.woff", "Inter-Medium.woff2"} {
		if err := os.WriteFile(filepath.Join(dir, n), readFixture(t, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewScanner(s, NewSfntParser()).Scan(ctx, addRoot(t, s, dir))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 || rep.Errored != 0 {
		t.Fatalf("report = %+v, want Scanned 2 Errored 0", rep)
	}
	for ext, want := range map[string]string{"woff": "woff", "woff2": "woff2"} {
		f, err := s.GetFontByPathFace(ctx, absPath(t, dir, "Inter-Medium."+ext), 0)
		if err != nil {
			t.Fatal(err)
		}
		if f.Format != want || f.Family == "" || f.ParseError != "" {
			t.Errorf("%s: format %q family %q parseErr %q", ext, f.Format, f.Family, f.ParseError)
		}
	}
}

func TestSyncSystemFontsIncludesTTC(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	ttf := readFixture(t, "Inter-Medium.ttf")
	path := filepath.Join(dir, "cambria.ttc")
	if err := os.WriteFile(path, buildTTC(t, ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := NewScanner(s, NewSfntParser()).SyncSystemFonts(ctx,
		[]SystemFontFile{{Path: path, Source: store.FontSourceSystemHKLM}}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2", rep.Scanned)
	}
	rows, err := s.ListFontsByPath(ctx, path)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %d err %v, want 2", len(rows), err)
	}
	for _, r := range rows {
		if r.Source != store.FontSourceSystemHKLM {
			t.Errorf("face %d source = %q", r.FaceIndex, r.Source)
		}
	}
}

func TestScanUppercaseExtension(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	files := map[string]string{"A.TTF": "Inter-Medium.ttf", "B.WOFF2": "Inter-Medium.woff2"}
	for n, fixture := range files {
		if err := os.WriteFile(filepath.Join(dir, n), readFixture(t, fixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewScanner(s, NewSfntParser()).Scan(ctx, addRoot(t, s, dir))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 || rep.Errored != 0 {
		t.Errorf("report = %+v, want Scanned 2 Errored 0", rep)
	}
}

type countingParserInternal struct {
	Parser
	calls int
}

func (p *countingParserInternal) Parse(path string) ([]FontRecord, error) {
	p.calls++
	return p.Parser.Parse(path)
}

func TestShrunkTTCNotReparsedOnLaterScans(t *testing.T) {
	ctx := context.Background()
	s := openScanStore(t)
	dir := t.TempDir()
	ttf := readFixture(t, "Inter-Medium.ttf")
	path := filepath.Join(dir, "pair.ttc")
	if err := os.WriteFile(path, buildTTC(t, ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	root := addRoot(t, s, dir)
	cp := &countingParserInternal{Parser: NewSfntParser()}
	sc := NewScanner(s, cp)
	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buildTTC(t, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Scan(ctx, root); err != nil { // shrink pass
		t.Fatal(err)
	}

	cp.calls = 0
	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if cp.calls != 0 || rep.Skipped != 1 || rep.Scanned != 0 || rep.Missing != 0 {
		t.Errorf("second rescan: parser calls %d, report %+v; want 0 calls, Skipped 1", cp.calls, rep)
	}
	abs := absPath(t, dir, "pair.ttc")
	if f1, err := s.GetFontByPathFace(ctx, abs, 1); err != nil || f1.Available {
		t.Errorf("stale face 1: %+v err %v, want present and unavailable", f1.Available, err)
	}

	// Growing back to two faces revives face 1.
	if err := os.WriteFile(path, buildTTC(t, ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	if f1, err := s.GetFontByPathFace(ctx, abs, 1); err != nil || !f1.Available {
		t.Errorf("regrown face 1 available = %v err %v, want true", f1.Available, err)
	}
}
