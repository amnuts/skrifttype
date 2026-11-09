package fonthelper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font/sfnt"

	"github.com/amnuts/skrifttype/pkg/helperproto"
)

// The helper runs elevated but takes its instructions from a file that a
// non-elevated process wrote, so it trusts nothing in the request:
//
//   - install only copies files that parse as TrueType/OpenType fonts, into
//     the system fonts directory, never overwriting an existing file or
//     registry value;
//   - uninstall only touches files directly inside the system fonts
//     directory, and only their own registry values;
//   - the result file must sit next to the request file and is created
//     exclusively (it never overwrites anything).

// maxFontSize bounds how much an elevated process will read per file.
const maxFontSize = 64 << 20

// system is the privileged surface the helper uses. The Windows
// implementation is in system_windows.go; tests use a fake.
type system interface {
	// FontsDir is the system fonts directory (%WINDIR%\Fonts).
	FontsDir() string
	// FontValues returns the HKLM fonts key as value name -> data.
	FontValues() (map[string]string, error)
	SetFontValue(name, data string) error
	DeleteFontValue(name string) error
	// AddFontResource / RemoveFontResource load or unload the font for
	// the current session (the registry covers future sessions).
	AddFontResource(path string) error
	RemoveFontResource(path string) error
}

func run(req helperproto.Request, sys system) helperproto.Result {
	res := helperproto.Result{Op: req.Op, Items: make([]helperproto.ResultItem, 0, len(req.Files))}
	if len(req.Files) == 0 {
		res.Error = "no files given"
		return res
	}
	var do func(string, system) helperproto.ResultItem
	switch req.Op {
	case helperproto.OpInstallSystem:
		do = install
	case helperproto.OpUninstallSystem:
		do = uninstall
	default:
		res.Error = fmt.Sprintf("unknown op %q", req.Op)
		return res
	}
	res.Success = true
	for _, f := range req.Files {
		item := do(f, sys)
		if item.Error != "" {
			res.Success = false
		}
		res.Items = append(res.Items, item)
	}
	if !res.Success {
		res.Error = "one or more fonts failed"
	}
	return res
}

func install(src string, sys system) helperproto.ResultItem {
	item := helperproto.ResultItem{SourcePath: src}
	fail := func(format string, a ...any) helperproto.ResultItem {
		item.Error = fmt.Sprintf(format, a...)
		return item
	}

	if !filepath.IsAbs(src) {
		return fail("path must be absolute")
	}
	src = filepath.Clean(src)
	kind, ok := registryKind(src)
	if !ok {
		return fail("only .ttf and .otf files can be installed")
	}
	data, err := readFont(src)
	if err != nil {
		return fail("%v", err)
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		return fail("not a valid font file: %v", err)
	}
	valueName := fontDisplayName(f, src) + " (" + kind + ")"

	values, err := sys.FontValues()
	if err != nil {
		return fail("reading installed fonts: %v", err)
	}
	for name := range values {
		if strings.EqualFold(name, valueName) {
			return fail("a font called %q is already installed", valueName)
		}
	}

	base := filepath.Base(src)
	if reservedName(base) {
		return fail("%s is not a usable file name", base)
	}
	dest := filepath.Join(sys.FontsDir(), base)
	if err := writeExclusive(dest, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fail("a font file named %s is already installed", base)
		}
		return fail("copying into the fonts folder: %v", err)
	}
	// Windows stores the bare file name for fonts inside the fonts folder.
	if err := sys.SetFontValue(valueName, base); err != nil {
		_ = os.Remove(dest)
		return fail("registering the font: %v", err)
	}
	item.InstalledAs = dest
	item.RegistryKey = valueName
	// Make it available now; the registry entry covers future sessions,
	// so a failure here is not fatal.
	_ = sys.AddFontResource(dest)
	return item
}

func uninstall(target string, sys system) helperproto.ResultItem {
	item := helperproto.ResultItem{SourcePath: target}
	fail := func(format string, a ...any) helperproto.ResultItem {
		item.Error = fmt.Sprintf(format, a...)
		return item
	}

	if !filepath.IsAbs(target) {
		return fail("path must be absolute")
	}
	target = filepath.Clean(target)
	if !strings.EqualFold(filepath.Dir(target), filepath.Clean(sys.FontsDir())) {
		return fail("only fonts in the system fonts folder can be uninstalled")
	}
	if _, ok := registryKind(target); !ok {
		return fail("only .ttf and .otf fonts can be uninstalled")
	}

	values, err := sys.FontValues()
	if err != nil {
		return fail("reading installed fonts: %v", err)
	}
	base := filepath.Base(target)
	var valueName string
	for name, data := range values {
		if strings.EqualFold(data, base) || strings.EqualFold(filepath.Clean(data), target) {
			valueName = name
			break
		}
	}
	if valueName == "" {
		return fail("%s is not registered as an installed font", base)
	}

	_ = sys.RemoveFontResource(target)
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Usually "in use": re-register the session copy so nothing is
		// half-removed, and let the user retry after closing apps.
		_ = sys.AddFontResource(target)
		return fail("the font file is in use; close apps using it and try again (%v)", err)
	}
	if err := sys.DeleteFontValue(valueName); err != nil {
		return fail("removing the registry entry: %v", err)
	}
	item.RegistryKey = valueName
	return item
}

// reservedName reports DOS device names (CON, NUL, COM1, ...), which
// Windows may resolve to devices even with an extension.
func reservedName(base string) bool {
	stem := strings.ToUpper(strings.TrimSpace(strings.SplitN(base, ".", 2)[0]))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return true
	}
	return false
}

// registryKind maps an extension to the suffix Windows uses in the
// fonts registry value name.
func registryKind(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf":
		return "TrueType", true
	case ".otf":
		return "OpenType", true
	}
	return "", false
}

func readFont(path string) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening font: %v", err)
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading font: %v", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() > maxFontSize {
		return nil, errors.New("font file is too large")
	}
	return io.ReadAll(io.LimitReader(fh, maxFontSize))
}

// fontDisplayName is the full font name (name ID 4), falling back to
// family + subfamily, then the file name.
func fontDisplayName(f *sfnt.Font, path string) string {
	var buf sfnt.Buffer
	clean := func(id sfnt.NameID) string {
		s, _ := f.Name(&buf, id)
		return strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, strings.TrimSpace(s))
	}
	if n := clean(sfnt.NameIDFull); n != "" {
		return n
	}
	fam, sub := clean(sfnt.NameIDFamily), clean(sfnt.NameIDSubfamily)
	if fam != "" {
		return strings.TrimSpace(fam + " " + sub)
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// writeExclusive creates path (failing if anything exists there) and
// writes data to it.
func writeExclusive(path string, data []byte) error {
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := fh.Write(data); err != nil {
		fh.Close()
		_ = os.Remove(path)
		return err
	}
	if err := fh.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// validateResultPath enforces that the result file lives next to the
// request file, is a .json file, and is outside the Windows directory.
func validateResultPath(requestPath, resultPath, windowsDir string) error {
	if !filepath.IsAbs(resultPath) || !filepath.IsAbs(requestPath) {
		return errors.New("paths must be absolute")
	}
	resultPath, requestPath = filepath.Clean(resultPath), filepath.Clean(requestPath)
	if !strings.EqualFold(filepath.Ext(resultPath), ".json") {
		return errors.New("result file must be .json")
	}
	resDir, err := resolvedDir(resultPath)
	if err != nil {
		return err
	}
	reqDir, err := resolvedDir(requestPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(resDir, reqDir) {
		return errors.New("result file must be next to the request file")
	}
	if windowsDir != "" {
		// Resolve it like resDir so short (8.3) names and links compare equal.
		if resolved, err := filepath.EvalSymlinks(windowsDir); err == nil {
			windowsDir = resolved
		}
		win :=strings.ToLower(filepath.Clean(windowsDir)) + string(filepath.Separator)
		if strings.HasPrefix(strings.ToLower(resDir)+string(filepath.Separator), win) {
			return errors.New("result file may not be inside the Windows directory")
		}
	}
	return nil
}

// resolvedDir returns the symlink/junction-resolved parent directory.
func resolvedDir(path string) (string, error) {
	d, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", filepath.Dir(path), err)
	}
	return filepath.Clean(d), nil
}
