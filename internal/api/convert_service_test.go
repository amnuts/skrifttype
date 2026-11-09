package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// convertEnv scans a folder containing one woff2 file and returns the
// service plus the id of its only face.
func convertEnv(t *testing.T) (*ConvertService, *env, int64) {
	t.Helper()
	e, dir := newEnv(t)
	if err := os.WriteFile(filepath.Join(dir, "w.woff2"), fixtureBytes(t, "Inter-Medium.woff2"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	f := e.onlyFamily()
	return NewConvertService(e.st), e, f.Styles[0].ID
}

func TestConvertServiceUsesSavedFolder(t *testing.T) {
	svc, _, id := convertEnv(t)
	out := filepath.Join(t.TempDir(), "saved")
	if err := svc.SetConvertFolder(out); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetConvertFolder()
	if err != nil || got != out {
		t.Fatalf("GetConvertFolder = %q, %v", got, err)
	}
	res, err := svc.ConvertFont(id, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Err != "" || filepath.Dir(res[0].Path) != out {
		t.Fatalf("results = %+v", res)
	}
	if _, err := os.Stat(res[0].Path); err != nil {
		t.Error(err)
	}
}

func TestConvertServiceRememberSaves(t *testing.T) {
	svc, _, id := convertEnv(t)
	out := t.TempDir()
	if _, err := svc.ConvertFont(id, true, out, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetConvertFolder(); got != "" {
		t.Errorf("folder saved without remember: %q", got)
	}
	if _, err := svc.ConvertFont(id, true, out, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetConvertFolder(); got != out {
		t.Errorf("folder = %q, want %q", got, out)
	}
}

func TestConvertServiceRememberSkippedOnFailure(t *testing.T) {
	svc, _, id := convertEnv(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(blocker, "sub")
	res, err := svc.ConvertFont(id, false, bad, true)
	if err != nil || len(res) != 1 || res[0].Err == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got, _ := svc.GetConvertFolder(); got != "" {
		t.Errorf("failed conversion saved folder %q", got)
	}
}

func TestConvertServiceNoFolderErrors(t *testing.T) {
	svc, _, id := convertEnv(t)
	_, err := svc.ConvertFont(id, false, "", false)
	if err == nil || !strings.Contains(err.Error(), "no conversion folder") {
		t.Fatalf("err = %v", err)
	}
}

func TestConvertServiceTTCSingleVersusAllFaces(t *testing.T) {
	e, dir := newEnv(t)
	ttf := fixtureBytes(t, "Inter-Medium.ttf")
	if err := os.WriteFile(filepath.Join(dir, "pair.ttc"), ttcOf(ttf, patchFamilyName(t, ttf)), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	fonts, err := e.lib.ListFonts(FontQueryDTO{Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	for _, fam := range fonts {
		for _, s := range fam.Styles {
			id = s.ID
		}
	}
	svc := NewConvertService(e.st)
	one, err := svc.ConvertFont(id, false, t.TempDir(), false)
	if err != nil || len(one) != 1 {
		t.Fatalf("one: %+v %v", one, err)
	}
	all, err := svc.ConvertFont(id, true, t.TempDir(), false)
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %+v %v", all, err)
	}
}

func TestInWatchedRoot(t *testing.T) {
	e, _ := newEnv(t)
	base := t.TempDir()
	root := filepath.Join(base, "Fonts")
	sibling := filepath.Join(base, "Fonts2")
	disabledDir := filepath.Join(base, "Off")
	for _, d := range []string{root, sibling, disabledDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.settings.AddWatchedRoot(root, true); err != nil {
		t.Fatal(err)
	}
	e.lib.wait()
	off, err := e.settings.AddWatchedRoot(disabledDir, true)
	if err != nil {
		t.Fatal(err)
	}
	e.lib.wait()
	if err := e.settings.SetWatchedRootEnabled(off.ID, false); err != nil {
		t.Fatal(err)
	}
	e.lib.wait()

	svc := NewConvertService(e.st)
	cases := []struct {
		dir  string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "sub", "deeper"), true},
		{root + string(filepath.Separator), true},
		{sibling, false},
		{base, false},
		{disabledDir, false},
	}
	for _, c := range cases {
		got, err := svc.InWatchedRoot(c.dir)
		if err != nil || got != c.want {
			t.Errorf("InWatchedRoot(%q) = %v, %v; want %v", c.dir, got, err, c.want)
		}
	}
}
