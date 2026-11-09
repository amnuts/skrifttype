//go:build windows

package winfont

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// fontsRegistryKey is the registry path used by both HKLM and HKCU to
// list installed fonts. It has been stable since Windows NT.
const fontsRegistryKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`

type windowsSystemFontEnumerator struct{}

func newPlatformSystemFontEnumerator() SystemFontEnumerator {
	return &windowsSystemFontEnumerator{}
}

func (e *windowsSystemFontEnumerator) EnumerateSystemFonts() ([]SystemFont, error) {
	hklmDir, err := systemFontsDir()
	if err != nil {
		return nil, fmt.Errorf("resolving HKLM fonts dir: %w", err)
	}
	hkcuDir, err := userFontsDir()
	if err != nil {
		return nil, fmt.Errorf("resolving HKCU fonts dir: %w", err)
	}

	hklm, err := readFontKey(registry.LOCAL_MACHINE, hklmDir, SourceSystemHKLM)
	if err != nil {
		return nil, fmt.Errorf("reading HKLM fonts: %w", err)
	}
	// HKCU is optional — many users have no per-user fonts. A missing
	// key returns no entries and no error.
	hkcu, err := readFontKey(registry.CURRENT_USER, hkcuDir, SourceSystemHKCU)
	if err != nil {
		return nil, fmt.Errorf("reading HKCU fonts: %w", err)
	}

	out := make([]SystemFont, 0, len(hklm)+len(hkcu))
	out = append(out, hklm...)
	out = append(out, hkcu...)
	return out, nil
}

// readFontKey opens HKLM or HKCU at the standard fonts subkey and
// returns one SystemFont per registry value. Missing keys are not an
// error (HKCU may not have one); other registry errors are returned.
func readFontKey(root registry.Key, fontsDir string, src Source) ([]SystemFont, error) {
	k, err := registry.OpenKey(root, fontsRegistryKey, registry.READ)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("OpenKey: %w", err)
	}
	defer k.Close()

	names, err := k.ReadValueNames(0)
	if err != nil {
		return nil, fmt.Errorf("ReadValueNames: %w", err)
	}

	out := make([]SystemFont, 0, len(names))
	for _, name := range names {
		raw, _, err := k.GetStringValue(name)
		if err != nil {
			// Skip entries we can't read rather than failing the whole
			// enumeration. A future logging hook could record these.
			continue
		}
		out = append(out, SystemFont{
			DisplayName: name,
			FilePath:    resolveFontPath(raw, fontsDir),
			Source:      src,
		})
	}
	return out, nil
}

// resolveFontPath turns a registry value (which may be a bare filename
// or an absolute path) into an absolute path. If the value already
// looks absolute (drive-letter prefix), it is returned unchanged after
// filepath.Clean. Otherwise it is joined with fontsDir.
func resolveFontPath(raw, fontsDir string) string {
	if isAbsoluteWindowsRegistryPath(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Join(fontsDir, raw)
}

// isAbsoluteWindowsRegistryPath checks for a drive-letter prefix.
// filepath.IsAbs would also work here but its behaviour depends on
// the build OS, and we want to be unambiguous on Windows.
func isAbsoluteWindowsRegistryPath(p string) bool {
	if len(p) < 3 {
		return false
	}
	c := p[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return false
	}
	return p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// systemFontsDir returns %WINDIR%\Fonts.
func systemFontsDir() (string, error) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return "", fmt.Errorf("WINDIR environment variable is not set")
	}
	return filepath.Join(windir, "Fonts"), nil
}

// userFontsDir returns %LOCALAPPDATA%\Microsoft\Windows\Fonts.
func userFontsDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("LOCALAPPDATA environment variable is not set")
	}
	return filepath.Join(local, "Microsoft", "Windows", "Fonts"), nil
}
