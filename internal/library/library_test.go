package library_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
)

func TestLibrary_ScanAll_AggregatesAcrossRoots(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	dirA := t.TempDir()
	writeFonts(t, dirA, "a1.ttf", "a2.ttf")

	dirB := t.TempDir()
	writeFonts(t, dirB, "b1.ttf")

	if _, err := s.AddWatchedRoot(ctx, dirA, true); err != nil {
		t.Fatalf("AddWatchedRoot A: %v", err)
	}
	if _, err := s.AddWatchedRoot(ctx, dirB, true); err != nil {
		t.Fatalf("AddWatchedRoot B: %v", err)
	}

	// Use the scanner with a fake parser so this test doesn't need
	// real font fixtures (the end-to-end fixture-based test is below).
	lib := library.NewLibraryWithParser(s, &fakeParser{})
	rep, err := lib.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if rep.Scanned != 3 {
		t.Errorf("Scanned: got %d, want 3 across both roots", rep.Scanned)
	}
}

func TestLibrary_ScanAll_SkipsDisabledRoots(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	dir := t.TempDir()
	writeFonts(t, dir, "a.ttf")

	root, err := s.AddWatchedRoot(ctx, dir, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}
	if err := s.SetWatchedRootEnabled(ctx, root.ID, false); err != nil {
		t.Fatalf("SetWatchedRootEnabled: %v", err)
	}

	lib := library.NewLibraryWithParser(s, &fakeParser{})
	rep, err := lib.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if rep.Scanned != 0 {
		t.Errorf("expected disabled root to be skipped, got Scanned=%d", rep.Scanned)
	}
}

// TestLibrary_EndToEnd_WithSfntParser exercises the full chain with
// the real sfnt parser and the bundled Inter fixture. It validates
// that a scan populates the store with realistic metadata.
func TestLibrary_EndToEnd_WithSfntParser(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// Stage the fixture in a temp directory the watched root can point at.
	dir := t.TempDir()
	src, err := filepath.Abs(filepath.Join("testdata", "Inter-Medium.ttf"))
	if err != nil {
		t.Fatalf("abs fixture: %v", err)
	}
	dst := filepath.Join(dir, "Inter-Medium.ttf")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	if _, err := s.AddWatchedRoot(ctx, dir, true); err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}

	lib := library.New(s)
	rep, err := lib.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if rep.Scanned != 1 {
		t.Errorf("Scanned: got %d, want 1", rep.Scanned)
	}

	all, err := s.ListFonts(ctx, store.FontFilter{Source: store.FontSourceLibrary})
	if err != nil {
		t.Fatalf("ListFonts: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 font in store, got %d", len(all))
	}
	got := all[0]
	if !strings.Contains(strings.ToLower(got.Family), "inter") {
		t.Errorf("Family: got %q, expected to contain 'inter'", got.Family)
	}
	if got.Format != "ttf" {
		t.Errorf("Format: got %q, want ttf", got.Format)
	}
	if got.Source != store.FontSourceLibrary {
		t.Errorf("Source: got %q, want library", got.Source)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
