// Package convert unpacks web fonts (WOFF/WOFF2) and splits font
// collections (TTC/OTC) into standalone desktop font files. It runs only on
// explicit user request and never modifies the source file.
package convert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amnuts/skrifttype/internal/library"
	"golang.org/x/image/font/sfnt"
)

// maxNameLen caps the base name (without suffix or extension) of an output
// file so the full path stays comfortably within Windows limits.
const maxNameLen = 120

// Result is the outcome of converting one face. Err is empty on success.
type Result struct {
	FaceIndex int
	Path      string
	Err       string
}

// Faces writes the requested faces of srcPath into outDir as standalone
// .ttf/.otf files, returning one Result per requested face. A nil
// faceIndexes means every face. Existing files are never overwritten: a
// colliding name gets a -2, -3, ... suffix. Failures of individual faces
// are reported in Result.Err; an error is returned only when the source
// cannot be read or understood.
func Faces(srcPath string, faceIndexes []int, outDir string) ([]Result, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	faces, _, err := library.SplitFaces(data)
	if err != nil {
		return nil, err
	}
	if faceIndexes == nil {
		faceIndexes = make([]int, len(faces))
		for i := range faces {
			faceIndexes[i] = i
		}
	}
	stem := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))

	results := make([]Result, 0, len(faceIndexes))
	mkErr := os.MkdirAll(outDir, 0o755)
	for _, idx := range faceIndexes {
		r := Result{FaceIndex: idx}
		switch {
		case idx < 0 || idx >= len(faces):
			r.Err = fmt.Sprintf("face index %d out of range (have %d)", idx, len(faces))
		case mkErr != nil:
			r.Err = fmt.Sprintf("create folder: %v", mkErr)
		default:
			path, err := writeFace(outDir, baseName(faces[idx], stem), library.OutlineExt(faces[idx]), faces[idx])
			if err != nil {
				r.Err = err.Error()
			} else {
				r.Path = path
			}
		}
		results = append(results, r)
	}
	return results, nil
}

// writeFace creates a new file for face, trying -2, -3, ... on name
// collisions. O_EXCL makes the no-overwrite guarantee atomic.
func writeFace(dir, base, ext string, face []byte) (string, error) {
	for n := 1; n < 10000; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		path := filepath.Join(dir, name+ext)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(face); err != nil {
			f.Close()
			os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", errors.New("could not find a free file name")
}

// baseName picks a file name for a face: PostScript name, else
// "family-style", else the source file stem.
func baseName(face []byte, stem string) string {
	var candidates []string
	if f, err := sfnt.Parse(face); err == nil {
		var buf sfnt.Buffer
		get := func(id sfnt.NameID) string {
			s, _ := f.Name(&buf, id)
			return strings.TrimSpace(s)
		}
		candidates = append(candidates, get(sfnt.NameIDPostScript))
		fam, sub := get(sfnt.NameIDTypographicFamily), get(sfnt.NameIDTypographicSubfamily)
		if fam == "" {
			fam = get(sfnt.NameIDFamily)
		}
		if sub == "" {
			sub = get(sfnt.NameIDSubfamily)
		}
		if fam != "" && sub != "" {
			candidates = append(candidates, fam+"-"+sub)
		} else {
			candidates = append(candidates, fam)
		}
	}
	candidates = append(candidates, stem)
	for _, c := range candidates {
		if s := sanitize(c); s != "" {
			return s
		}
	}
	return "font"
}

var reservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

// sanitize makes s safe as a Windows file name base. It returns "" when
// nothing usable remains.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	out := []rune(b.String())
	if len(out) > maxNameLen {
		out = out[:maxNameLen]
	}
	res := strings.TrimLeft(strings.TrimRight(string(out), ". "), " ")
	if strings.Trim(res, "_") == "" {
		return ""
	}
	head := strings.ToUpper(res)
	if i := strings.IndexByte(head, '.'); i >= 0 {
		head = head[:i]
	}
	head = strings.TrimRight(head, " ")
	if reservedNames[head] || isNumbered(head, "COM") || isNumbered(head, "LPT") {
		res = "_" + res
	}
	return res
}

func isNumbered(name, prefix string) bool {
	return len(name) == len(prefix)+1 && strings.HasPrefix(name, prefix) && name[len(prefix)] >= '1' && name[len(prefix)] <= '9'
}
