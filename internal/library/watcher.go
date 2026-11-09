package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/amnuts/skrifttype/internal/store"
)

// DefaultDebounce is how long the watcher waits for a burst of file
// events (copying a folder of fonts, unzipping a family) to settle
// before asking for a rescan.
const DefaultDebounce = 1500 * time.Millisecond

// Watcher watches every enabled watched root for font files being
// added, changed, removed or renamed, and calls onChange once per burst
// of activity. It does not scan anything itself: the callback (a
// rescan) is cheap because unchanged files are skipped.
//
// fsnotify watches single directories, so recursive roots are covered
// by watching each subdirectory, and directories created later are
// added as they appear.
type Watcher struct {
	store    *store.Store
	onChange func()
	debounce time.Duration
	log      *slog.Logger

	fw *fsnotify.Watcher

	mu        sync.Mutex
	roots     []store.WatchedRoot // enabled roots, absolute paths
	watched   map[string]struct{} // directories currently watched
	timer     *time.Timer
	closeOnce sync.Once
}

// NewWatcher creates a watcher. Call Sync to (re)load the roots and Run
// to start delivering events.
func NewWatcher(st *store.Store, onChange func(), debounce time.Duration, log *slog.Logger) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	return &Watcher{store: st, onChange: onChange, debounce: debounce, log: log, fw: fw, watched: map[string]struct{}{}}, nil
}

// Sync reloads the enabled roots from the store and adjusts the set of
// watched directories. Call it after watched roots change.
func (w *Watcher) Sync(ctx context.Context) error {
	roots, err := w.store.ListEnabledWatchedRoots(ctx)
	if err != nil {
		return err
	}
	for i := range roots {
		if abs, err := filepath.Abs(roots[i].Path); err == nil {
			roots[i].Path = abs
		}
	}

	want := map[string]struct{}{}
	for _, r := range roots {
		for _, d := range dirsUnder(r) {
			want[d] = struct{}{}
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.roots = roots
	for d := range w.watched {
		if _, ok := want[d]; !ok {
			_ = w.fw.Remove(d)
			delete(w.watched, d)
		}
	}
	for d := range want {
		w.addLocked(d)
	}
	return nil
}

func (w *Watcher) addLocked(dir string) {
	if _, ok := w.watched[dir]; ok {
		return
	}
	if err := w.fw.Add(dir); err != nil {
		// Offline drive, permission denied, ...: the next launch's
		// rescan still catches up, so log and carry on.
		w.log.Warn("watcher: cannot watch directory", "dir", dir, "err", err)
		return
	}
	w.watched[dir] = struct{}{}
}

// dirsUnder lists the root itself plus, for recursive roots, every
// readable subdirectory.
func dirsUnder(r store.WatchedRoot) []string {
	if !r.Recursive {
		return []string{r.Path}
	}
	var out []string
	_ = filepath.WalkDir(r.Path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != r.Path {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	if len(out) == 0 {
		out = []string{r.Path}
	}
	return out
}

// Run delivers events until ctx is cancelled or Close is called.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.Close()
			return
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			// Typically a buffer overflow during a huge copy: we may
			// have missed events, so rescan to be safe.
			w.log.Warn("watcher: error", "err", err)
			w.kick()
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	if ev.Op == fsnotify.Chmod {
		return
	}
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			// A new folder may already contain fonts (moved in whole).
			if root, ok := w.rootFor(ev.Name); ok && root.Recursive {
				w.mu.Lock()
				for _, d := range dirsUnder(store.WatchedRoot{Path: ev.Name, Recursive: true}) {
					w.addLocked(d)
				}
				w.mu.Unlock()
			}
			w.kick()
			return
		}
	}
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		w.mu.Lock()
		if _, ok := w.watched[ev.Name]; ok {
			// A watched directory went away; fsnotify drops the watch.
			delete(w.watched, ev.Name)
			w.mu.Unlock()
			w.kick()
			return
		}
		w.mu.Unlock()
	}
	if isFontPath(ev.Name) {
		w.kick()
	}
}

// rootFor returns the enabled root containing path.
func (w *Watcher) rootFor(path string) (store.WatchedRoot, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range w.roots {
		if rel, err := filepath.Rel(r.Path, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return r, true
		}
	}
	return store.WatchedRoot{}, false
}

// kick (re)starts the debounce timer.
func (w *Watcher) kick() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, w.onChange)
}

// Close stops watching. Safe to call more than once.
func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.mu.Unlock()
		_ = w.fw.Close()
	})
}
