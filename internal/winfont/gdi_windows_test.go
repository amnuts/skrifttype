//go:build windows

package winfont_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnuts/skrifttype/internal/winfont"
)

// arialPath returns an absolute path to arial.ttf in the Windows fonts
// directory. arial.ttf ships with every Windows installation, so the
// test does not require a custom fixture. The test skips gracefully
// if the file is missing (e.g. on a stripped-down Server SKU).
func arialPath(t *testing.T) string {
	t.Helper()
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	p := filepath.Join(windir, "Fonts", "arial.ttf")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("arial.ttf not found at %s: %v", p, err)
	}
	return p
}

func TestGDI_AddRemoveRoundTrip(t *testing.T) {
	g := winfont.NewGDI()
	path := arialPath(t)

	n, err := g.AddFontResource(path)
	if err != nil {
		t.Fatalf("AddFontResource: %v", err)
	}
	if n < 1 {
		t.Fatalf("AddFontResource: expected at least 1 face, got %d", n)
	}

	if err := g.RemoveFontResource(path); err != nil {
		t.Fatalf("RemoveFontResource: %v", err)
	}
}

func TestGDI_AddBadPathReturnsError(t *testing.T) {
	g := winfont.NewGDI()
	_, err := g.AddFontResource(`C:\does\not\exist\fake.ttf`)
	if err == nil {
		t.Fatal("AddFontResource: expected error for nonexistent path, got nil")
	}
	// Per Plan 1's whole-plan review: assert the error message names the
	// syscall so a regression that strips the wrap is caught.
	if !strings.Contains(err.Error(), "AddFontResourceExW") {
		t.Fatalf("expected error message to mention AddFontResourceExW, got %q", err.Error())
	}
	// Also confirm we did not regress to ErrUnsupportedPlatform (the
	// stub error from Task 1) on Windows.
	if errors.Is(err, winfont.ErrUnsupportedPlatform) {
		t.Fatalf("got ErrUnsupportedPlatform on Windows; impl regressed to stub")
	}
}

func TestGDI_BroadcastFontChangeReturnsNoError(t *testing.T) {
	g := winfont.NewGDI()
	path := arialPath(t)

	if _, err := g.AddFontResource(path); err != nil {
		t.Fatalf("AddFontResource: %v", err)
	}
	t.Cleanup(func() { _ = g.RemoveFontResource(path) })

	if err := g.BroadcastFontChange(); err != nil {
		t.Fatalf("BroadcastFontChange: %v", err)
	}
}
