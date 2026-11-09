package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
)

// countingParser counts Parse calls so tests can tell a skip from a re-parse.
type countingParser struct {
	fakeParser
	calls int
}

func (p *countingParser) Parse(path string) ([]library.FontRecord, error) {
	p.calls++
	return p.fakeParser.Parse(path)
}

func TestRescan_SyncsSystemFontsReadOnlyAndMarksMissing(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	paths := writeFonts(t, dir, "arial.ttf", "times.ttf", "cambria.ttc")

	lib := library.NewLibraryWithParser(s, &fakeParser{})
	files := []library.SystemFontFile{
		{Path: paths[0], Source: store.FontSourceSystemHKLM},
		{Path: paths[1], Source: store.FontSourceSystemHKCU},
		{Path: paths[2], Source: store.FontSourceSystemHKLM}, // .ttc: catalogued like any other font
	}
	rep, err := lib.Rescan(ctx, files)
	if err != nil {
		t.Fatalf("Rescan: %v", err)
	}
	if rep.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3 (ttc included)", rep.Scanned)
	}
	got, err := s.GetFontByPathFace(ctx, paths[1], 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != store.FontSourceSystemHKCU || got.WatchedRootID != 0 {
		t.Errorf("system font row = source %q root %d", got.Source, got.WatchedRootID)
	}

	// Uninstalled from the OS: next sync marks it missing.
	rep, err = lib.Rescan(ctx, files[:1])
	if err != nil {
		t.Fatal(err)
	}
	if rep.Missing != 2 {
		t.Errorf("Missing = %d, want 2 (hkcu ttf + ttc)", rep.Missing)
	}
	got, _ = s.GetFontByPathFace(ctx, paths[1], 0)
	if got.Available {
		t.Error("removed system font still available")
	}
}

func TestRescan_ForcesReparseOnceAfterParserUpgrade(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	writeFonts(t, dir, "a.ttf")
	if _, err := s.AddWatchedRoot(ctx, dir, true); err != nil {
		t.Fatal(err)
	}

	// Simulate a catalogue built by an older parser.
	p := &countingParser{}
	if _, err := library.NewLibraryWithParser(s, p).ScanAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPreference(ctx, "library.parserVersion", "1"); err != nil {
		t.Fatal(err)
	}

	lib := library.NewLibraryWithParser(s, p)
	before := p.calls
	if _, err := lib.Rescan(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if p.calls != before+1 {
		t.Errorf("upgrade rescan parsed %d files, want 1 (forced)", p.calls-before)
	}
	before = p.calls
	if _, err := lib.Rescan(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if p.calls != before {
		t.Errorf("second rescan re-parsed %d unchanged files", p.calls-before)
	}
}

func TestScanner_UnchangedButPreviouslyMissingIsRestored(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	dir := t.TempDir()
	paths := writeFonts(t, dir, "a.ttf")
	root, _ := s.AddWatchedRoot(ctx, dir, true)
	lib := library.NewLibraryWithParser(s, &fakeParser{})
	if _, err := lib.ScanRoot(ctx, root); err != nil {
		t.Fatal(err)
	}
	// Move away and back with the same mtime (e.g. a network drive
	// that was briefly offline).
	info, _ := os.Stat(paths[0])
	tmp := filepath.Join(t.TempDir(), "a.ttf")
	if err := os.Rename(paths[0], tmp); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.ScanRoot(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, paths[0]); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(paths[0], info.ModTime(), info.ModTime())
	if _, err := lib.ScanRoot(ctx, root); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetFontByPathFace(ctx, paths[0], 0)
	if !got.Available {
		t.Error("font that came back is still marked missing")
	}
}
