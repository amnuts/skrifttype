package api

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amnuts/skrifttype/internal/convert"
	"github.com/amnuts/skrifttype/internal/store"
)

// convertFolderPref is the preferences key holding the last-used
// conversion output folder.
const convertFolderPref = "convert_folder"

// ConvertResultDTO is the frontend-facing outcome of converting one face.
type ConvertResultDTO struct {
	FaceIndex int    `json:"faceIndex"`
	Path      string `json:"path"`
	Err       string `json:"err"`
}

// ConvertService converts web fonts and splits collections into plain
// font files on explicit user request. Bound via application.NewService.
type ConvertService struct {
	store *store.Store
}

// NewConvertService constructs the service.
func NewConvertService(st *store.Store) *ConvertService {
	return &ConvertService{store: st}
}

// GetConvertFolder returns the saved conversion folder, or "" if unset.
func (s *ConvertService) GetConvertFolder() (string, error) {
	v, _, err := s.store.GetPreference(context.Background(), convertFolderPref)
	return v, err
}

// SetConvertFolder saves the conversion folder.
func (s *ConvertService) SetConvertFolder(dir string) error {
	return s.store.SetPreference(context.Background(), convertFolderPref, dir)
}

// ConvertFont converts the font with the given id into outDir (or the
// saved folder when outDir is empty). allFaces converts every face of the
// font's file; otherwise only that font's own face. With remember, outDir
// is saved as the conversion folder after at least one face was written.
func (s *ConvertService) ConvertFont(fontID int64, allFaces bool, outDir string, remember bool) ([]ConvertResultDTO, error) {
	f, err := s.store.GetFontByID(context.Background(), fontID)
	if err != nil {
		return nil, err
	}
	if outDir == "" {
		if outDir, err = s.GetConvertFolder(); err != nil {
			return nil, err
		}
		if outDir == "" {
			return nil, errors.New("no conversion folder")
		}
	}
	var faces []int
	if !allFaces {
		faces = []int{f.FaceIndex}
	}
	res, err := convert.Faces(f.Path, faces, outDir)
	if err != nil {
		return nil, err
	}
	out := make([]ConvertResultDTO, 0, len(res))
	wrote := false
	for _, r := range res {
		out = append(out, ConvertResultDTO{FaceIndex: r.FaceIndex, Path: r.Path, Err: r.Err})
		wrote = wrote || r.Err == ""
	}
	if remember && wrote {
		if err := s.SetConvertFolder(outDir); err != nil {
			return out, err
		}
	}
	return out, nil
}

// InWatchedRoot reports whether dir is equal to or inside any enabled
// watched root, so the UI can tell the user converted fonts will be
// picked up by the next scan.
func (s *ConvertService) InWatchedRoot(dir string) (bool, error) {
	roots, err := s.store.ListEnabledWatchedRoots(context.Background())
	if err != nil {
		return false, err
	}
	d, err := normalisePath(dir)
	if err != nil {
		return false, err
	}
	for _, r := range roots {
		root, err := normalisePath(r.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, d)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true, nil
		}
	}
	return false, nil
}

func normalisePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}
