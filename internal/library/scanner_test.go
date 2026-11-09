package library_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// fakeParser returns one FontRecord per path, deriving Family from
// the filename so each fixture is unique (avoids the Plan-2b-defect
// where shared default fields silently broke an FTS test).
type fakeParser struct {
	failPaths map[string]bool
}

func (p *fakeParser) Parse(path string) ([]library.FontRecord, error) {
	if p.failPaths != nil && p.failPaths[path] {
		return nil, errFakeParser
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return []library.FontRecord{{
		Family:    "Family-" + stem,
		Subfamily: "Regular",
		FullName:  "Family-" + stem + " Regular",
		Format:    "ttf",
	}}, nil
}

var errFakeParser = errFake("fake parser failure")

type errFake string

func (e errFake) Error() string { return string(e) }

// writeFonts creates n empty .ttf files inside dir and returns their
// absolute paths.
func writeFonts(t *testing.T, dir string, names ...string) []string {
	t.Helper()
	var paths []string
	for _, n := range names {
		full := filepath.Join(dir, n)
		if err := os.WriteFile(full, []byte("not a real font"), 0644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
		abs, err := filepath.Abs(full)
		if err != nil {
			t.Fatalf("abs %s: %v", full, err)
		}
		paths = append(paths, abs)
	}
	return paths
}

func TestScanner_InsertsAllFontsInRoot(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	writeFonts(t, dir, "a.ttf", "b.otf", "ignored.txt")

	root, err := s.AddWatchedRoot(ctx, dir, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}

	sc := library.NewScanner(s, &fakeParser{})
	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if rep.Scanned != 2 {
		t.Errorf("Scanned: got %d, want 2", rep.Scanned)
	}
	if rep.Skipped != 0 {
		t.Errorf("Skipped: got %d, want 0", rep.Skipped)
	}
	if rep.Errored != 0 {
		t.Errorf("Errored: got %d, want 0", rep.Errored)
	}
	if rep.Missing != 0 {
		t.Errorf("Missing: got %d, want 0", rep.Missing)
	}

	all, err := s.ListFonts(ctx, store.FontFilter{})
	if err != nil {
		t.Fatalf("ListFonts: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 fonts in store, got %d", len(all))
	}
}

func TestScanner_SkipsUnchangedOnRescan(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	writeFonts(t, dir, "x.ttf", "y.ttf")

	root, _ := s.AddWatchedRoot(ctx, dir, true)
	sc := library.NewScanner(s, &fakeParser{})

	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if rep.Skipped != 2 {
		t.Errorf("Skipped: got %d, want 2 (both files unchanged)", rep.Skipped)
	}
	if rep.Scanned != 0 {
		t.Errorf("Scanned: got %d, want 0", rep.Scanned)
	}
}

func TestScanner_RescansChangedFile(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	paths := writeFonts(t, dir, "z.ttf")

	root, _ := s.AddWatchedRoot(ctx, dir, true)
	sc := library.NewScanner(s, &fakeParser{})

	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatalf("first Scan: %v", err)
	}

	// Modify the file: change content AND bump mtime by 2 seconds so
	// the unix-second mtime comparison registers the change.
	if err := os.WriteFile(paths[0], []byte("changed"), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(paths[0], future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if rep.Scanned != 1 {
		t.Errorf("Scanned: got %d, want 1", rep.Scanned)
	}
	if rep.Skipped != 0 {
		t.Errorf("Skipped: got %d, want 0", rep.Skipped)
	}
}

func TestScanner_MarksMissingAsUnavailable(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	paths := writeFonts(t, dir, "keeper.ttf", "doomed.ttf")

	root, _ := s.AddWatchedRoot(ctx, dir, true)
	sc := library.NewScanner(s, &fakeParser{})

	if _, err := sc.Scan(ctx, root); err != nil {
		t.Fatalf("first Scan: %v", err)
	}

	// Delete one font from disk.
	if err := os.Remove(paths[1]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if rep.Missing != 1 {
		t.Errorf("Missing: got %d, want 1", rep.Missing)
	}
	if rep.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1 (the surviving font)", rep.Skipped)
	}

	// The doomed font must now be unavailable in the store.
	gone, err := s.GetFontByPathFace(ctx, paths[1], 0)
	if err != nil {
		t.Fatalf("GetFontByPathFace doomed: %v", err)
	}
	if gone.Available {
		t.Error("expected doomed font to have Available=false")
	}
}

func TestScanner_ParseErrorRecordedAsRow(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	paths := writeFonts(t, dir, "broken.ttf")

	root, _ := s.AddWatchedRoot(ctx, dir, true)
	parser := &fakeParser{failPaths: map[string]bool{paths[0]: true}}
	sc := library.NewScanner(s, parser)

	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if rep.Errored != 1 {
		t.Errorf("Errored: got %d, want 1", rep.Errored)
	}

	// Even on parse failure, a row is inserted with ParseError set so
	// the UI can surface it.
	stored, err := s.GetFontByPathFace(ctx, paths[0], 0)
	if err != nil {
		t.Fatalf("GetFontByPathFace broken: %v", err)
	}
	if stored.ParseError == "" {
		t.Error("expected ParseError to be non-empty on parse failure")
	}
}

func TestScanner_NonRecursiveStaysAtRoot(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	writeFonts(t, dir, "top.ttf")

	subDir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	writeFonts(t, subDir, "nested.ttf")

	// Recursive=false: only top.ttf should be picked up.
	root, _ := s.AddWatchedRoot(ctx, dir, false)
	sc := library.NewScanner(s, &fakeParser{})
	rep, err := sc.Scan(ctx, root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if rep.Scanned != 1 {
		t.Errorf("Scanned: got %d, want 1 (recursive=false should ignore sub/nested.ttf)", rep.Scanned)
	}
}
