//go:build windows

package winfont

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsGDI is the real Win32 implementation of the GDI interface.
// It holds lazy handles to gdi32 and user32 — see the package-level
// var block below.
type windowsGDI struct{}

func newPlatformGDI() GDI { return &windowsGDI{} }

var (
	gdi32                     = windows.NewLazySystemDLL("gdi32.dll")
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procAddFontResourceExW    = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceExW = gdi32.NewProc("RemoveFontResourceExW")
	procPostMessageW          = user32.NewProc("PostMessageW")
)

// Win32 constants used by this package. Names follow Go-idiomatic
// lower-camel style; values are the exact Win32 SDK values.
const (
	hwndBroadcast = 0xFFFF
	wmFontChange  = 0x001D
)

// AddFontResource calls AddFontResourceExW with dwFlags=0 (public,
// enumerable). The path MUST be absolute — see GDI interface doc.
func (g *windowsGDI) AddFontResource(path string) (FaceCount, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("AddFontResourceExW utf16: %w", err)
	}
	r, _, callErr := procAddFontResourceExW.Call(
		uintptr(unsafe.Pointer(p)),
		0, // dwFlags = 0  -> public, enumerable
		0, // pdv reserved, must be NULL
	)
	if r == 0 {
		return 0, fmt.Errorf("AddFontResourceExW: %w", callErr)
	}
	return FaceCount(r), nil
}

// RemoveFontResource calls RemoveFontResourceExW. Per Win32 semantics
// the path must match the one originally passed to AddFontResource
// (compared by file name).
func (g *windowsGDI) RemoveFontResource(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("RemoveFontResourceExW utf16: %w", err)
	}
	r, _, callErr := procRemoveFontResourceExW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		0,
	)
	if r == 0 {
		return fmt.Errorf("RemoveFontResourceExW: %w", callErr)
	}
	return nil
}

// BroadcastFontChange posts WM_FONTCHANGE to HWND_BROADCAST. Uses
// PostMessage (fire-and-forget) rather than SendMessage so a hung
// target window cannot block this process.
func (g *windowsGDI) BroadcastFontChange() error {
	r, _, callErr := procPostMessageW.Call(
		hwndBroadcast,
		wmFontChange,
		0,
		0,
	)
	if r == 0 {
		return fmt.Errorf("PostMessageW(WM_FONTCHANGE): %w", callErr)
	}
	return nil
}
