package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amnuts/skrifttype/internal/library"
)

const testDebounce = 150 * time.Millisecond

type watchEnv struct {
	t     *testing.T
	calls chan struct{}
	w     *library.Watcher
}

func startWatcher(t *testing.T, dir string, recursive bool) (*watchEnv, func(enabled bool)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := openTestStore(t)
	root, err := s.AddWatchedRoot(ctx, dir, recursive)
	if err != nil {
		t.Fatal(err)
	}
	e := &watchEnv{t: t, calls: make(chan struct{}, 16)}
	e.w, err = library.NewWatcher(s, func() { e.calls <- struct{}{} }, testDebounce, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	go e.w.Run(ctx)
	setEnabled := func(enabled bool) {
		if err := s.SetWatchedRootEnabled(ctx, root.ID, enabled); err != nil {
			t.Fatal(err)
		}
		if err := e.w.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return e, setEnabled
}

// expect waits for exactly n callbacks, then checks no more arrive.
func (e *watchEnv) expect(n int) {
	e.t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-e.calls:
		case <-time.After(3 * time.Second):
			e.t.Fatalf("got %d change callbacks, want %d", i, n)
		}
	}
	select {
	case <-e.calls:
		e.t.Fatalf("got more than %d change callbacks", n)
	case <-time.After(3 * testDebounce):
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcher_BurstOfFontsIsOneChange(t *testing.T) {
	dir := t.TempDir()
	e, _ := startWatcher(t, dir, true)
	for _, n := range []string{"a.ttf", "b.otf", "c.TTF", "d.ttf"} {
		write(t, filepath.Join(dir, n))
	}
	e.expect(1)
}

func TestWatcher_IgnoresNonFontFiles(t *testing.T) {
	dir := t.TempDir()
	e, _ := startWatcher(t, dir, true)
	write(t, filepath.Join(dir, "readme.txt"))
	write(t, filepath.Join(dir, "license.pdf"))
	e.expect(0)
}

func TestWatcher_RemovalAndRenameAreChanges(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.ttf")
	write(t, f)
	e, _ := startWatcher(t, dir, true)
	if err := os.Rename(f, filepath.Join(dir, "b.ttf")); err != nil {
		t.Fatal(err)
	}
	e.expect(1)
	if err := os.Remove(filepath.Join(dir, "b.ttf")); err != nil {
		t.Fatal(err)
	}
	e.expect(1)
}

func TestWatcher_FollowsNewSubdirectoriesInRecursiveRoots(t *testing.T) {
	dir := t.TempDir()
	e, _ := startWatcher(t, dir, true)
	sub := filepath.Join(dir, "family")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e.expect(1) // the new folder itself
	write(t, filepath.Join(sub, "x.ttf"))
	e.expect(1) // a font inside it
}

func TestWatcher_NonRecursiveIgnoresSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e, _ := startWatcher(t, dir, false)
	write(t, filepath.Join(sub, "x.ttf"))
	e.expect(0)
}

func TestWatcher_DisabledRootIsUnwatched(t *testing.T) {
	dir := t.TempDir()
	e, setEnabled := startWatcher(t, dir, true)
	setEnabled(false)
	write(t, filepath.Join(dir, "a.ttf"))
	e.expect(0)
	setEnabled(true)
	write(t, filepath.Join(dir, "b.ttf"))
	e.expect(1)
}
