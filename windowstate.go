package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowState is the persisted geometry of the main window, stored as JSON in
// %LOCALAPPDATA%\skrifttype\window.json so size and position survive restarts.
type windowState struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	X      int  `json:"x"`
	Y      int  `json:"y"`
	HasPos bool `json:"hasPos"`
}

// appDataDir is where the catalogue and window state live. Set
// SKRIFTTYPE_DATA_DIR to use a throwaway profile (development, testing).
func appDataDir() string {
	if d := os.Getenv("SKRIFTTYPE_DATA_DIR"); d != "" {
		return d
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if d, err := os.UserConfigDir(); err == nil {
			base = d
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "skrifttype")
}

func windowStateFile() string { return filepath.Join(appDataDir(), "window.json") }

func loadWindowState() (windowState, bool) {
	var ws windowState
	b, err := os.ReadFile(windowStateFile())
	if err != nil {
		return windowState{}, false
	}
	if err := json.Unmarshal(b, &ws); err != nil || ws.Width <= 0 || ws.Height <= 0 {
		return windowState{}, false
	}
	return ws, true
}

// visibleOn reports whether enough of the saved window's title bar would land
// inside one of the given work areas for the user to grab it. Saved positions
// can go stale (monitor unplugged or rearranged, resolution changed), and
// restoring one blindly can put the window entirely off-screen.
func visibleOn(ws windowState, workAreas []application.Rect) bool {
	const (
		titleBar   = 32  // height of the strip that must be reachable
		minVisible = 100 // horizontal pixels of it that must be on one screen
		minHeight  = 16  // vertical pixels of it that must be on one screen
	)
	for _, wa := range workAreas {
		w := min(ws.X+ws.Width, wa.X+wa.Width) - max(ws.X, wa.X)
		h := min(ws.Y+titleBar, wa.Y+wa.Height) - max(ws.Y, wa.Y)
		if w >= minVisible && h >= minHeight {
			return true
		}
	}
	return false
}

// windowPlacement re-centres the main window when its restored position is
// not visible on any connected monitor. It is a service only because
// ServiceStartup is the one hook that runs after Wails has enumerated the
// screens but before it creates the window.
type windowPlacement struct {
	app *application.App
	win *application.WebviewWindow
	ws  windowState
	log *slog.Logger
}

func (p *windowPlacement) ServiceStartup(context.Context, application.ServiceOptions) error {
	var areas []application.Rect
	for _, s := range p.app.Screen.GetAll() {
		areas = append(areas, s.WorkArea)
	}
	if len(areas) > 0 && !visibleOn(p.ws, areas) {
		p.log.Info("saved window position is off-screen; centring", "x", p.ws.X, "y", p.ws.Y)
		p.win.Center() // before creation this just switches InitialPosition
	}
	return nil
}

// windowStateSaver debounces disk writes so a drag (which fires many move/resize
// events) results in a single write shortly after the user stops.
type windowStateSaver struct {
	mu      sync.Mutex
	pending windowState
	timer   *time.Timer
}

// record captures the latest geometry (read on the caller's thread) and
// schedules a debounced flush to disk.
func (s *windowStateSaver) record(ws windowState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = ws
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(400*time.Millisecond, s.flush)
}

func (s *windowStateSaver) flush() {
	s.mu.Lock()
	ws := s.pending
	s.mu.Unlock()
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		return
	}
	if b, err := json.Marshal(ws); err == nil {
		_ = os.WriteFile(windowStateFile(), b, 0o644)
	}
}

// windowsOptions applies two development switches:
//   - SKRIFTTYPE_DATA_DIR also isolates the WebView2 profile (localStorage,
//     cache) under that directory, so a throwaway profile is fully separate.
//   - SKRIFTTYPE_WEBVIEW_DEBUG_PORT exposes the Chrome DevTools protocol on
//     that localhost port, for inspecting or scripting the packaged app.
func windowsOptions() application.WindowsOptions {
	var o application.WindowsOptions
	if os.Getenv("SKRIFTTYPE_DATA_DIR") != "" {
		o.WebviewUserDataPath = filepath.Join(appDataDir(), "webview")
	}
	if port := os.Getenv("SKRIFTTYPE_WEBVIEW_DEBUG_PORT"); port != "" {
		o.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + port}
	}
	return o
}

// openLog sends slog output to <data dir>/skrifttype.log (truncated each
// launch) as well as stderr: a GUI-subsystem binary has no console, so
// without a file nothing it logs is ever visible.
func openLog() (*slog.Logger, func()) {
	f, err := os.Create(filepath.Join(appDataDir(), "skrifttype.log"))
	if err != nil {
		return slog.Default(), func() {}
	}
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, f), nil))
	slog.SetDefault(logger)
	return logger, func() { _ = f.Close() }
}
