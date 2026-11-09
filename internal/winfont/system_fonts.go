package winfont

// Source identifies where a system-installed font was discovered.
// Library fonts (managed by skrifttype) use a different identifier
// stored in the SQLite layer.
type Source string

const (
	// SourceSystemHKLM identifies a font installed for all users
	// (registry key under HKEY_LOCAL_MACHINE).
	SourceSystemHKLM Source = "system-hklm"

	// SourceSystemHKCU identifies a font installed for the current
	// user only (registry key under HKEY_CURRENT_USER).
	SourceSystemHKCU Source = "system-hkcu"
)

// SystemFont is one row of the OS's installed-font lists. It is
// read-only from skrifttype's perspective — system fonts cannot be
// activated or deactivated via this package.
type SystemFont struct {
	// DisplayName is the registry value name as Windows stores it,
	// e.g. "Inter (TrueType)".
	DisplayName string

	// FilePath is the resolved absolute path to the font file. For
	// HKLM entries this is under %WINDIR%\Fonts; for HKCU it is under
	// %LOCALAPPDATA%\Microsoft\Windows\Fonts. If the registry value
	// already contains an absolute path, that path is preserved.
	FilePath string

	// Source records which registry key produced this entry.
	Source Source
}

// SystemFontEnumerator reads the OS's installed-font lists from the
// Windows registry. Implementations MUST resolve every entry to an
// absolute file path. Implementations SHOULD preserve duplicates
// across HKLM and HKCU as separate entries — deduplication is the
// caller's concern.
type SystemFontEnumerator interface {
	EnumerateSystemFonts() ([]SystemFont, error)
}

// NewSystemFontEnumerator returns a SystemFontEnumerator appropriate
// for the current platform.
func NewSystemFontEnumerator() SystemFontEnumerator {
	return newPlatformSystemFontEnumerator()
}
