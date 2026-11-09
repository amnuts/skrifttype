// Package winfont contains the only code in skrifttype permitted to
// import golang.org/x/sys/windows. Everything else takes the GDI and
// SystemFontEnumerator interfaces declared here so that tests can
// substitute fakes and the package compiles cross-platform.
package winfont

import "errors"

// ErrUnsupportedPlatform is returned by the non-Windows stub
// implementation of every method in this package. It is a sentinel —
// callers should test with errors.Is.
var ErrUnsupportedPlatform = errors.New("winfont: not supported on this platform")

// FaceCount is the number of font faces that a single file contributed
// when loaded. For .ttf/.otf this is typically 1; for .ttc/.otc it can
// be greater. Returned by GDI.AddFontResource.
type FaceCount = int

// GDI is the thin wrapper over Win32 font load/unload and the
// WM_FONTCHANGE broadcast. The interface is deliberately minimal so
// tests can fake it with a hand-written struct.
//
// Path arguments MUST be absolute. The wrappers do not call
// filepath.Abs — that responsibility lives one layer up (the activation
// manager, which sources paths from SQLite where they are already
// canonical). Passing a relative path produces a Win32 error tied to
// the process's current working directory at call time.
type GDI interface {
	// AddFontResource calls AddFontResourceExW with dwFlags=0, making
	// the font visible to all processes on the next BroadcastFontChange.
	// Returns the number of faces added, or an error.
	AddFontResource(path string) (FaceCount, error)

	// RemoveFontResource calls RemoveFontResourceExW. The path must be
	// the same one passed to AddFontResource (Windows compares by file
	// name). On success the font's reference count is decremented; the
	// font becomes invisible to other processes only when the count
	// reaches zero AND the next BroadcastFontChange fires.
	RemoveFontResource(path string) error

	// BroadcastFontChange posts WM_FONTCHANGE to HWND_BROADCAST so
	// running applications re-enumerate their font lists. Implementations
	// MUST use PostMessage (fire-and-forget), not SendMessage.
	BroadcastFontChange() error
}

// NewGDI returns a GDI implementation appropriate for the current
// platform. On Windows this is a real GDI wrapper; on every other
// platform every method returns ErrUnsupportedPlatform.
func NewGDI() GDI {
	return newPlatformGDI()
}
