//go:build windows

package winfont_test

import (
	"os"
	"testing"

	"github.com/amnuts/skrifttype/internal/winfont"
)

func TestSystemFontEnumerator_FindsAtLeastOneHKLMFont(t *testing.T) {
	e := winfont.NewSystemFontEnumerator()
	fonts, err := e.EnumerateSystemFonts()
	if err != nil {
		t.Fatalf("EnumerateSystemFonts: %v", err)
	}
	if len(fonts) == 0 {
		t.Fatal("EnumerateSystemFonts returned 0 fonts; expected the Windows base font set")
	}

	var hasHKLM bool
	for _, f := range fonts {
		if f.Source == winfont.SourceSystemHKLM {
			hasHKLM = true
			break
		}
	}
	if !hasHKLM {
		t.Fatal("expected at least one HKLM-sourced font; HKLM is the machine-wide registry and is always populated")
	}
}

func TestSystemFontEnumerator_PathsAreAbsoluteAndResolvable(t *testing.T) {
	e := winfont.NewSystemFontEnumerator()
	fonts, err := e.EnumerateSystemFonts()
	if err != nil {
		t.Fatalf("EnumerateSystemFonts: %v", err)
	}

	// At least one of the returned paths must point at a real file.
	// We allow some entries to be unresolvable (font files can be
	// removed without cleaning up the registry), but if none resolve
	// the implementation is broken.
	var resolved int
	for _, f := range fonts {
		if !isAbsoluteWindowsPath(f.FilePath) {
			t.Errorf("font %q FilePath=%q: expected absolute path", f.DisplayName, f.FilePath)
			continue
		}
		if _, err := os.Stat(f.FilePath); err == nil {
			resolved++
		}
	}
	if resolved == 0 {
		t.Fatalf("none of %d enumerated font paths resolve to a real file", len(fonts))
	}
}

// isAbsoluteWindowsPath returns true if p starts with a drive letter
// followed by a colon and a path separator (the only form Windows GDI
// accepts here). filepath.IsAbs is intentionally not used because its
// behaviour depends on the build platform.
func isAbsoluteWindowsPath(p string) bool {
	if len(p) < 3 {
		return false
	}
	c := p[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return false
	}
	return p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}
