package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/api"
	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
	"github.com/amnuts/skrifttype/internal/winfont"
	"github.com/amnuts/skrifttype/pkg/helperproto"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// main is the application's entry point. It opens the SQLite store, restores
// previously-active fonts, wires the backend services, creates the main
// window (restoring its saved geometry), and runs the application. On quit it
// unloads every font it activated; the next launch restores them.
func main() {
	// Open the SQLite store (creates the data dir + runs migrations).
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		log.Fatal(err)
	}
	logger, closeLog := openLog()
	defer closeLog()
	st, err := store.Open(context.Background(), filepath.Join(appDataDir(), "skrifttype.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	// ctx bounds background work (rescans) and is cancelled on shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := activation.New(st, winfont.NewGDI(), logger)
	// unload stops background rescans (so none can reconcile fonts back
	// in) and releases every font this process activated. Safe to call
	// more than once: the second call finds nothing loaded.
	unload := func() {
		cancel()
		if err := mgr.UnloadAll(); err != nil {
			logger.Warn("unloading fonts", "err", err)
		}
	}
	// Closing the last window makes Run return without running the
	// OnShutdown hook (Wails v3 beta.34), so unload on the way out too.
	defer unload()
	cat := api.NewCatalog(st)

	// Services are constructed before the app so they can be registered in
	// Options; events are emitted through app once it exists.
	var app *application.App
	emit := func(name string, data any) {
		if app != nil {
			app.Event.Emit(name, data)
		}
	}
	libSvc := api.NewLibraryService(ctx, st, library.New(st), cat, mgr,
		winfont.NewSystemFontEnumerator(), emit, logger)

	// "Install for everyone" goes through font-helper.exe (elevated via
	// UAC), shipped next to this executable on Windows.
	var runHelper api.HelperRunner
	if helperExe, ok := winfont.HelperPath(); ok && runtime.GOOS == "windows" {
		runHelper = func(op helperproto.Op, files []string) (helperproto.Result, error) {
			return winfont.RunHelper(helperExe, op, files)
		}
	}

	app = application.New(application.Options{
		Name:        "skrifttype",
		Description: "A local-first font library that activates fonts on demand",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// Serves /fontfile/<id> so previews can render fonts that are
			// not installed.
			Middleware: api.FontFileMiddleware(st),
		},
		Services: []application.Service{
			application.NewService(libSvc),
			application.NewService(api.NewActivationService(st, cat, mgr, emit)),
			application.NewService(api.NewCollectionService(st, cat, mgr)),
			application.NewService(api.NewInstallService(st, cat, mgr, libSvc.Rescan, runHelper, nil)),
			application.NewService(api.NewConvertService(st)),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Windows: windowsOptions(),
		OnShutdown: func() {
			logger.Info("shutting down")
			unload()
		},
	})

	// Re-activate whatever was active when the app last closed, before the
	// UI appears (spec §6 "Restore on launch").
	if failed, err := mgr.Reconcile(ctx); err != nil {
		logger.Warn("restoring active fonts", "err", err)
	} else if len(failed) > 0 {
		logger.Warn("some fonts could not be restored", "count", len(failed))
	}

	// Fonts left loaded by earlier runs (killed before they could unload) are
	// invisible to the manager; unload them in the background.
	go func() {
		n, err := mgr.SweepStale(ctx)
		switch {
		case err != nil:
			logger.Warn("unloading stale fonts", "err", err)
		case n > 0:
			logger.Info("unloaded stale fonts", "count", n)
		}
	}()

	// Create a new window. Size/position are restored from disk if previously
	// saved, otherwise a sensible default is used.
	opts := application.WebviewWindowOptions{
		Title:     "skrifttype",
		Width:     1180,
		Height:    760,
		MinWidth:  900,
		MinHeight: 560,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	}
	if ws, ok := loadWindowState(); ok {
		opts.Width, opts.Height = ws.Width, ws.Height
		if ws.HasPos {
			opts.X, opts.Y = ws.X, ws.Y
			opts.InitialPosition = application.WindowXY
		}
	}
	win := app.Window.NewWithOptions(opts)

	// Live folder watching: a burst of font files appearing, changing or
	// vanishing in a watched folder triggers one rescan.
	watcher, err := library.NewWatcher(st, libSvc.Rescan, library.DefaultDebounce, logger)
	if err != nil {
		logger.Warn("folder watching unavailable", "err", err)
	}
	syncWatcher := func() {
		if watcher != nil {
			if err := watcher.Sync(ctx); err != nil {
				logger.Warn("watcher sync", "err", err)
			}
		}
	}

	// Settings needs the window for the folder picker, so it is registered
	// after the window exists. Folder changes rescan and re-point the watcher.
	app.RegisterService(application.NewService(api.NewSettingsService(st, app, win, func() {
		libSvc.Rescan()
		syncWatcher()
	})))

	// Catch up with changes made while the app was closed (new or removed
	// files in watched folders, newly installed system fonts), then watch.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		libSvc.Rescan()
		syncWatcher()
		if watcher != nil {
			go watcher.Run(ctx)
		}
	})

	// Persist window geometry (debounced) whenever the user resizes or moves it.
	saver := &windowStateSaver{}
	persist := func(*application.WindowEvent) {
		w, h := win.Size()
		x, y := win.Position()
		saver.record(windowState{Width: w, Height: h, X: x, Y: y, HasPos: true})
	}
	win.OnWindowEvent(events.Common.WindowDidResize, persist)
	win.OnWindowEvent(events.Common.WindowDidMove, persist)

	// Run the application. This blocks until the application has been exited.
	if err := app.Run(); err != nil {
		logger.Error("application exited with error", "err", err)
	}
}
