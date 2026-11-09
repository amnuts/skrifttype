package fonthelper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnuts/skrifttype/pkg/helperproto"
)

type fakeSystem struct {
	dir     string
	values  map[string]string
	loaded  map[string]int
	failSet bool
}

func newFake(t *testing.T) *fakeSystem {
	return &fakeSystem{dir: t.TempDir(), values: map[string]string{"Arial (TrueType)": "arial.ttf"}, loaded: map[string]int{}}
}

func (f *fakeSystem) FontsDir() string { return f.dir }
func (f *fakeSystem) FontValues() (map[string]string, error) {
	out := map[string]string{}
	for k, v := range f.values {
		out[k] = v
	}
	return out, nil
}
func (f *fakeSystem) SetFontValue(n, d string) error {
	if f.failSet {
		return os.ErrPermission
	}
	f.values[n] = d
	return nil
}
func (f *fakeSystem) DeleteFontValue(n string) error    { delete(f.values, n); return nil }
func (f *fakeSystem) AddFontResource(p string) error    { f.loaded[p]++; return nil }
func (f *fakeSystem) RemoveFontResource(p string) error { f.loaded[p]--; return nil }

// fixture copies the Inter test font into a user folder.
func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "Inter-Medium.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallThenUninstall(t *testing.T) {
	sys := newFake(t)
	src := fixture(t, "Inter-Medium.ttf")

	res := run(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{src}}, sys)
	if !res.Success || len(res.Items) != 1 {
		t.Fatalf("install result = %+v", res)
	}
	it := res.Items[0]
	dest := filepath.Join(sys.dir, "Inter-Medium.ttf")
	if it.InstalledAs != dest || !strings.HasSuffix(it.RegistryKey, " (TrueType)") {
		t.Errorf("item = %+v", it)
	}
	if sys.values[it.RegistryKey] != "Inter-Medium.ttf" {
		t.Errorf("registry = %v", sys.values)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("font not copied: %v", err)
	}
	if sys.loaded[dest] != 1 {
		t.Errorf("font not loaded for the session")
	}

	// Installing again must not overwrite anything.
	res = run(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{src}}, sys)
	if res.Success || res.Items[0].Error == "" {
		t.Errorf("second install succeeded: %+v", res)
	}

	res = run(helperproto.Request{Op: helperproto.OpUninstallSystem, Files: []string{dest}}, sys)
	if !res.Success {
		t.Fatalf("uninstall = %+v", res)
	}
	if _, ok := sys.values[it.RegistryKey]; ok {
		t.Error("registry value left behind")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("font file left behind")
	}
	if sys.loaded[dest] != 0 {
		t.Error("font still loaded for the session")
	}
}

func TestInstallRejectsNonFonts(t *testing.T) {
	sys := newFake(t)
	dir := t.TempDir()
	notFont := filepath.Join(dir, "evil.ttf")
	_ = os.WriteFile(notFont, []byte("MZ this is an executable"), 0o644)
	txt := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(txt, []byte("hello"), 0o644)
	dev := fixture(t, "CON.ttf")

	for _, p := range []string{notFont, txt, "relative.ttf", dir, dev, `C:\does\not\exist.ttf`} {
		res := run(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{p}}, sys)
		if res.Success || res.Items[0].Error == "" {
			t.Errorf("install %q accepted: %+v", p, res)
		}
	}
	entries, _ := os.ReadDir(sys.dir)
	if len(entries) != 0 {
		t.Errorf("fonts dir written: %v", entries)
	}
}

func TestInstallRollsBackFileWhenRegistryFails(t *testing.T) {
	sys := newFake(t)
	sys.failSet = true
	res := run(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{fixture(t, "a.ttf")}}, sys)
	if res.Success {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(sys.dir, "a.ttf")); !os.IsNotExist(err) {
		t.Error("copied file not rolled back")
	}
}

func TestUninstallOnlyTouchesRegisteredFontsInFontsDir(t *testing.T) {
	sys := newFake(t)
	outside := fixture(t, "a.ttf")
	unregistered := filepath.Join(sys.dir, "b.ttf")
	_ = os.WriteFile(unregistered, []byte("x"), 0o644)
	sneaky := filepath.Join(sys.dir, "..", filepath.Base(outside))

	for _, p := range []string{outside, unregistered, sneaky, filepath.Join(sys.dir, "arial.dll")} {
		res := run(helperproto.Request{Op: helperproto.OpUninstallSystem, Files: []string{p}}, sys)
		if res.Success {
			t.Errorf("uninstall %q succeeded", p)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("file outside the fonts dir was removed")
	}
	if _, err := os.Stat(unregistered); err != nil {
		t.Error("unregistered file was removed")
	}
}

func TestRealMain_WritesResultNextToRequestOnly(t *testing.T) {
	sys := newFake(t)
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request.json")
	write := func(r helperproto.Request) {
		b, _ := json.Marshal(r)
		if err := os.WriteFile(reqPath, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	resPath := filepath.Join(dir, "result.json")
	write(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{fixture(t, "a.ttf")}, ResultTo: resPath})
	if code := realMain([]string{reqPath}, sys); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var res helperproto.Result
	b, _ := os.ReadFile(resPath)
	if err := json.Unmarshal(b, &res); err != nil || !res.Success {
		t.Fatalf("result = %s (%v)", b, err)
	}

	// Never overwrites an existing file.
	if code := realMain([]string{reqPath}, sys); code != 2 {
		t.Errorf("rewriting an existing result: exit %d, want 2", code)
	}

	elsewhere := filepath.Join(t.TempDir(), "result.json")
	for _, bad := range []string{elsewhere, filepath.Join(dir, "result.exe"), "result.json"} {
		write(helperproto.Request{Op: helperproto.OpInstallSystem, Files: []string{"x"}, ResultTo: bad})
		if code := realMain([]string{reqPath}, sys); code != 2 {
			t.Errorf("ResultTo %q accepted (exit %d)", bad, code)
		}
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Error("wrote a result outside the request directory")
	}

	if code := realMain(nil, sys); code != 2 {
		t.Errorf("no args: exit %d", code)
	}
}

func TestValidateResultPath_RejectsWindowsDir(t *testing.T) {
	win := t.TempDir()
	req := filepath.Join(win, "System32", "req.json")
	_ = os.MkdirAll(filepath.Dir(req), 0o755)
	if err := validateResultPath(req, filepath.Join(filepath.Dir(req), "r.json"), win); err == nil {
		t.Error("result inside the Windows directory accepted")
	}
}
