package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/store"
	"github.com/amnuts/skrifttype/internal/winfont"
	"github.com/amnuts/skrifttype/pkg/helperproto"
)

// installedPrefKey records the system font paths skrifttype installed,
// as a JSON array. Only these are offered for "uninstall for everyone":
// the app must never remove fonts Windows or other software installed.
const installedPrefKey = "helper.installedPaths"

// HelperRunner performs a system-wide operation (winfont.RunHelper in
// production; a fake in tests).
type HelperRunner func(op helperproto.Op, files []string) (helperproto.Result, error)

// InstallFailureDTO is one file the helper could not process.
type InstallFailureDTO struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// InstallResultDTO summarises a system-wide install or uninstall.
type InstallResultDTO struct {
	Succeeded []string            `json:"succeeded"` // file names
	Failed    []InstallFailureDTO `json:"failed"`
}

// InstallService installs library fonts for all users via the elevated
// helper (one UAC prompt per action) and uninstalls fonts it installed.
type InstallService struct {
	store     *store.Store
	cat       *Catalog
	mgr       *activation.Manager
	rescan    func()
	run       HelperRunner
	available func() bool
}

// NewInstallService wires the service. run may be nil when no helper is
// available; available reports whether system-wide installs can work.
func NewInstallService(st *store.Store, cat *Catalog, mgr *activation.Manager, rescan func(), run HelperRunner, available func() bool) *InstallService {
	if available == nil {
		available = func() bool { return run != nil }
	}
	return &InstallService{store: st, cat: cat, mgr: mgr, rescan: rescan, run: run, available: available}
}

// CanInstallSystemWide reports whether this build can install fonts for
// all users (Windows, with font-helper.exe next to the app).
func (s *InstallService) CanInstallSystemWide() bool { return s.run != nil && s.available() }

// InstallForEveryone copies the family's font files into the Windows
// fonts folder for all users. The user sees one UAC prompt. The
// library copy is then deactivated (the installed copy replaces it).
func (s *InstallService) InstallForEveryone(familyID int64) (InstallResultDTO, error) {
	ctx := context.Background()
	if !s.CanInstallSystemWide() {
		return InstallResultDTO{}, errors.New("installing for all users is not available on this system")
	}
	idx, err := s.cat.index(ctx)
	if err != nil {
		return InstallResultDTO{}, err
	}
	fam, ok := idx.byID[familyID]
	if !ok {
		return InstallResultDTO{}, fmt.Errorf("unknown font %d", familyID)
	}
	if fam.dto.Source != store.FontSourceLibrary {
		return InstallResultDTO{}, errors.New("this font is already installed on the system")
	}
	var files []string
	var faces []int64
	var webFailed []InstallFailureDTO
	seenWeb := map[string]bool{}
	seenColl := map[string]bool{}
	for _, st := range fam.dto.Styles {
		if !st.Available {
			continue
		}
		if !st.Activatable {
			// Web fonts (WOFF/WOFF2) can't be installed on Windows. Skip
			// them before any UAC prompt; one entry per file.
			if !seenWeb[st.Path] {
				seenWeb[st.Path] = true
				webFailed = append(webFailed, InstallFailureDTO{File: filepath.Base(st.Path), Error: "web fonts cannot be installed on Windows"})
			}
			continue
		}
		if ext := strings.ToLower(filepath.Ext(st.Path)); ext == ".ttc" || ext == ".otc" {
			// The elevated helper accepts only .ttf/.otf, so a collection
			// would fail after the UAC prompt. Skip it up front.
			if !seenColl[st.Path] {
				seenColl[st.Path] = true
				webFailed = append(webFailed, InstallFailureDTO{File: filepath.Base(st.Path), Error: "font collections can't be installed for all users; convert them first"})
			}
			continue
		}
		files = append(files, st.Path)
		faces = append(faces, st.ID)
	}
	if len(files) == 0 {
		if len(webFailed) > 0 {
			return InstallResultDTO{Succeeded: []string{}, Failed: webFailed}, nil
		}
		return InstallResultDTO{}, errors.New("none of this family's font files could be found")
	}

	res, err := s.run(helperproto.OpInstallSystem, files)
	if err != nil {
		return InstallResultDTO{}, friendlyHelperError(err)
	}
	out, installed := summarise(res)
	out.Failed = append(out.Failed, webFailed...)
	if len(installed) > 0 {
		if err := s.updateInstalled(ctx, installed, nil); err != nil {
			return out, err
		}
		// The system copy is now always on; drop the manual activation
		// of the library copies that were installed (only those: a face
		// that failed must stay active). Items follow Files order.
		var installedFaces []int64
		for i, it := range res.Items {
			if i < len(faces) && it.Error == "" && it.InstalledAs != "" {
				installedFaces = append(installedFaces, faces[i])
			}
		}
		if _, err := s.mgr.DeactivateManual(ctx, installedFaces...); err != nil {
			return out, err
		}
		_ = s.mgr.Rebroadcast()
		s.cat.Invalidate()
		if s.rescan != nil {
			s.rescan() // picks up the new system fonts
		}
	}
	return out, nil
}

// UninstallForEveryone removes a family that skrifttype installed for
// all users. Other system fonts are refused.
func (s *InstallService) UninstallForEveryone(familyID int64) (InstallResultDTO, error) {
	ctx := context.Background()
	if !s.CanInstallSystemWide() {
		return InstallResultDTO{}, errors.New("uninstalling for all users is not available on this system")
	}
	idx, err := s.cat.index(ctx)
	if err != nil {
		return InstallResultDTO{}, err
	}
	fam, ok := idx.byID[familyID]
	if !ok {
		return InstallResultDTO{}, fmt.Errorf("unknown font %d", familyID)
	}
	mine, err := loadInstalled(ctx, s.store)
	if err != nil {
		return InstallResultDTO{}, err
	}
	var files []string
	for _, st := range fam.dto.Styles {
		if mine[pathKey(st.Path)] {
			files = append(files, st.Path)
		}
	}
	if len(files) == 0 {
		return InstallResultDTO{}, errors.New("skrifttype can only uninstall fonts it installed itself")
	}

	res, err := s.run(helperproto.OpUninstallSystem, files)
	if err != nil {
		return InstallResultDTO{}, friendlyHelperError(err)
	}
	out, _ := summarise(res)
	var removed []string
	for _, it := range res.Items {
		if it.Error == "" {
			removed = append(removed, it.SourcePath)
		}
	}
	if len(removed) > 0 {
		if err := s.updateInstalled(ctx, nil, removed); err != nil {
			return out, err
		}
		_ = s.mgr.Rebroadcast()
		s.cat.Invalidate()
		if s.rescan != nil {
			s.rescan()
		}
	}
	return out, nil
}

func summarise(res helperproto.Result) (InstallResultDTO, []string) {
	out := InstallResultDTO{Succeeded: []string{}, Failed: []InstallFailureDTO{}}
	var installed []string
	for _, it := range res.Items {
		name := filepath.Base(it.SourcePath)
		if it.Error != "" {
			out.Failed = append(out.Failed, InstallFailureDTO{File: name, Error: it.Error})
			continue
		}
		out.Succeeded = append(out.Succeeded, name)
		if it.InstalledAs != "" {
			installed = append(installed, it.InstalledAs)
		}
	}
	if len(res.Items) == 0 && res.Error != "" {
		out.Failed = append(out.Failed, InstallFailureDTO{Error: res.Error})
	}
	return out, installed
}

func friendlyHelperError(err error) error {
	if errors.Is(err, winfont.ErrElevationDeclined) {
		return errors.New("permission was declined, so nothing was installed for all users; you can still activate the font for yourself")
	}
	return err
}

func (s *InstallService) updateInstalled(ctx context.Context, add, remove []string) error {
	set, err := loadInstalled(ctx, s.store)
	if err != nil {
		return err
	}
	for _, p := range add {
		set[pathKey(p)] = true
	}
	for _, p := range remove {
		delete(set, pathKey(p))
	}
	list := make([]string, 0, len(set))
	for p := range set {
		list = append(list, p)
	}
	slices.Sort(list)
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.store.SetPreference(ctx, installedPrefKey, string(b))
}

// loadInstalled returns the set of paths (normalised by pathKey) that
// skrifttype installed system-wide.
func loadInstalled(ctx context.Context, st *store.Store) (map[string]bool, error) {
	v, ok, err := st.GetPreference(ctx, installedPrefKey)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	if !ok || v == "" {
		return set, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(v), &list); err != nil {
		return set, nil // corrupt value: treat as empty rather than fail
	}
	for _, p := range list {
		set[pathKey(p)] = true
	}
	return set, nil
}

// pathKey normalises a Windows path for set membership.
func pathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }
