// Package api hosts the Wails-bound services exposed to the frontend.
package api

import (
	"context"
	"path/filepath"

	"github.com/amnuts/skrifttype/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WatchedRootDTO is the frontend-facing shape of a watched folder.
type WatchedRootDTO struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	Enabled   bool   `json:"enabled"`
}

// SettingsService exposes watched-folder CRUD, preferences, and a native
// folder picker to the frontend. Bound via application.NewService.
type SettingsService struct {
	store *store.Store
	app   *application.App
	win   *application.WebviewWindow
	// rootsChanged runs after any successful watched-root change; main
	// wires it to LibraryService.Rescan. May be nil.
	rootsChanged func()
}

// NewSettingsService constructs the service. app/win are used only by
// PickFolder; rootsChanged (optional) is called after watched folders
// are added, removed, enabled or disabled.
func NewSettingsService(st *store.Store, app *application.App, win *application.WebviewWindow, rootsChanged func()) *SettingsService {
	return &SettingsService{store: st, app: app, win: win, rootsChanged: rootsChanged}
}

func (s *SettingsService) changed() {
	if s.rootsChanged != nil {
		s.rootsChanged()
	}
}

func toDTO(r store.WatchedRoot) WatchedRootDTO {
	return WatchedRootDTO{ID: r.ID, Path: r.Path, Recursive: r.Recursive, Enabled: r.Enabled}
}

// ListWatchedRoots returns all watched folders.
func (s *SettingsService) ListWatchedRoots() ([]WatchedRootDTO, error) {
	roots, err := s.store.ListWatchedRoots(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]WatchedRootDTO, 0, len(roots))
	for _, r := range roots {
		out = append(out, toDTO(r))
	}
	return out, nil
}

// AddWatchedRoot adds a folder to watch.
func (s *SettingsService) AddWatchedRoot(path string, recursive bool) (WatchedRootDTO, error) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	r, err := s.store.AddWatchedRoot(context.Background(), path, recursive)
	if err != nil {
		return WatchedRootDTO{}, err
	}
	s.changed()
	return toDTO(r), nil
}

// RemoveWatchedRoot removes a watched folder by id, dropping its fonts
// from the catalogue (and so from any collections and activations).
func (s *SettingsService) RemoveWatchedRoot(id int64) error {
	ctx := context.Background()
	if err := s.store.DeleteFontsInWatchedRoot(ctx, id); err != nil {
		return err
	}
	if err := s.store.RemoveWatchedRoot(ctx, id); err != nil {
		return err
	}
	s.changed()
	return nil
}

// SetWatchedRootEnabled enables/disables a watched folder. A disabled
// folder's fonts are hidden and unloaded but keep their collection
// memberships and activation intent for when it is re-enabled.
func (s *SettingsService) SetWatchedRootEnabled(id int64, enabled bool) error {
	if err := s.store.SetWatchedRootEnabled(context.Background(), id, enabled); err != nil {
		return err
	}
	s.changed()
	return nil
}

// PickFolder opens the native directory chooser. Returns "" if cancelled.
func (s *SettingsService) PickFolder() (string, error) {
	if s.app == nil {
		return "", nil
	}
	dlg := s.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose a font folder")
	if s.win != nil {
		dlg = dlg.AttachToWindow(s.win)
	}
	return dlg.PromptForSingleSelection()
}

// GetPreference returns the stored value, or "" if unset.
func (s *SettingsService) GetPreference(key string) (string, error) {
	v, _, err := s.store.GetPreference(context.Background(), key)
	return v, err
}

// SetPreference stores a preference value.
func (s *SettingsService) SetPreference(key, value string) error {
	return s.store.SetPreference(context.Background(), key, value)
}
