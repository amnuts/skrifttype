package api

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
	"github.com/amnuts/skrifttype/internal/winfont"
	"github.com/amnuts/skrifttype/pkg/helperproto"
)

type fakeHelper struct {
	fontsDir string
	calls    []helperproto.Request
	err      error
	failFile string
}

func (h *fakeHelper) run(op helperproto.Op, files []string) (helperproto.Result, error) {
	h.calls = append(h.calls, helperproto.Request{Op: op, Files: files})
	if h.err != nil {
		return helperproto.Result{}, h.err
	}
	res := helperproto.Result{Op: op, Success: true}
	for _, f := range files {
		it := helperproto.ResultItem{SourcePath: f}
		switch {
		case filepath.Base(f) == h.failFile:
			it.Error = "boom"
			res.Success = false
		case op == helperproto.OpInstallSystem:
			it.InstalledAs = filepath.Join(h.fontsDir, filepath.Base(f))
		}
		res.Items = append(res.Items, it)
	}
	return res, nil
}

func TestInstallService_InstallThenUninstall(t *testing.T) {
	e, dir := newEnv(t, "a.ttf")
	e.addRootAndScan(dir)
	fam := e.onlyFamily()
	if _, err := e.act.ActivateManual(fam.ID); err != nil {
		t.Fatal(err)
	}

	h := &fakeHelper{fontsDir: `C:\Windows\Fonts`}
	rescans := 0
	svc := NewInstallService(e.st, e.cat, e.act.mgr, func() { rescans++ }, h.run, nil)
	if !svc.CanInstallSystemWide() {
		t.Fatal("CanInstallSystemWide = false with a helper")
	}

	res, err := svc.InstallForEveryone(fam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Succeeded, []string{"a.ttf"}) || len(res.Failed) != 0 {
		t.Errorf("result = %+v", res)
	}
	if len(h.calls) != 1 || h.calls[0].Op != helperproto.OpInstallSystem || h.calls[0].Files[0] != fam.Styles[0].Path {
		t.Errorf("helper calls = %+v", h.calls)
	}
	if rescans != 1 {
		t.Errorf("rescans = %d, want 1", rescans)
	}
	if set, _ := e.act.GetActiveSet(); len(set) != 0 {
		t.Errorf("library copy still active after system install: %v", set)
	}

	// Simulate the rescan cataloguing the installed system copy.
	installedPath := filepath.Join(h.fontsDir, "a.ttf")
	if _, err := e.st.UpsertFont(e.ctx, store.Font{Path: installedPath, FileSize: 1, FileMtime: 1,
		Family: "Inter", Subfamily: "Medium", FullName: "Inter Medium", Format: "ttf",
		Source: store.FontSourceSystemHKLM, Available: true}); err != nil {
		t.Fatal(err)
	}
	e.cat.Invalidate()
	fonts, _ := e.lib.ListFonts(FontQueryDTO{})
	var sys *FontDTO
	for i := range fonts {
		if fonts[i].Source == store.FontSourceSystemHKLM {
			sys = &fonts[i]
		}
	}
	if sys == nil || !sys.InstalledByApp {
		t.Fatalf("installed system family not flagged: %+v", fonts)
	}

	if _, err := svc.UninstallForEveryone(sys.ID); err != nil {
		t.Fatal(err)
	}
	last := h.calls[len(h.calls)-1]
	if last.Op != helperproto.OpUninstallSystem || !slices.Equal(last.Files, []string{installedPath}) {
		t.Errorf("uninstall call = %+v", last)
	}
	e.cat.Invalidate()
	fonts, _ = e.lib.ListFonts(FontQueryDTO{})
	for _, f := range fonts {
		if f.InstalledByApp {
			t.Errorf("%s still flagged after uninstall", f.Family)
		}
	}
}

func TestInstallService_RefusesFontsItDidNotInstall(t *testing.T) {
	e, _ := newEnv(t)
	if _, err := e.st.UpsertFont(e.ctx, store.Font{Path: `C:\Windows\Fonts\arial.ttf`, FileSize: 1, FileMtime: 1,
		Family: "Arial", Subfamily: "Regular", FullName: "Arial", Format: "ttf",
		Source: store.FontSourceSystemHKLM, Available: true}); err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{}
	svc := NewInstallService(e.st, e.cat, e.act.mgr, nil, h.run, nil)
	fonts, _ := e.lib.ListFonts(FontQueryDTO{})
	if _, err := svc.UninstallForEveryone(fonts[0].ID); err == nil {
		t.Error("uninstalling a system font skrifttype did not install was allowed")
	}
	if _, err := svc.InstallForEveryone(fonts[0].ID); err == nil {
		t.Error("installing an already-system font was allowed")
	}
	if len(h.calls) != 0 {
		t.Errorf("helper was invoked: %+v", h.calls)
	}
}

func TestInstallService_DeclinedAndPartialFailures(t *testing.T) {
	e, dir := newEnv(t, "a.ttf", "b.ttf")
	e.addRootAndScan(dir)
	fam := e.onlyFamily()

	h := &fakeHelper{fontsDir: `C:\Windows\Fonts`, err: winfont.ErrElevationDeclined}
	svc := NewInstallService(e.st, e.cat, e.act.mgr, nil, h.run, nil)
	if _, err := svc.InstallForEveryone(fam.ID); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Errorf("declined UAC error = %v", err)
	}

	if _, err := e.act.ActivateManual(fam.ID); err != nil {
		t.Fatal(err)
	}
	h.err = nil
	h.failFile = "b.ttf"
	res, err := svc.InstallForEveryone(fam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Succeeded) != 1 || len(res.Failed) != 1 || res.Failed[0].File != "b.ttf" {
		t.Errorf("partial result = %+v", res)
	}
	// Only the installed face loses its library activation; the face
	// that failed to install must stay active.
	for _, st := range fam.Styles {
		active, _ := e.st.IsActive(e.ctx, st.ID)
		want := filepath.Base(st.Path) == "b.ttf"
		if active != want {
			t.Errorf("%s active = %v, want %v", filepath.Base(st.Path), active, want)
		}
	}

	noHelper := NewInstallService(e.st, e.cat, e.act.mgr, nil, nil, nil)
	if noHelper.CanInstallSystemWide() {
		t.Error("CanInstallSystemWide without a helper")
	}
	if _, err := noHelper.InstallForEveryone(fam.ID); err == nil {
		t.Error("install without a helper should fail")
	}
}

func TestInstallService_SkipsCollections(t *testing.T) {
	const msg = "font collections can't be installed for all users; convert them first"
	ttf := fixtureBytes(t, "Inter-Medium.ttf")

	// Collection-only family: no helper call (no UAC), one failure per file.
	e, dir := newEnv(t)
	if err := os.WriteFile(filepath.Join(dir, "pair.TTC"), ttcOf(ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	e.addRootAndScan(dir)
	fam := e.onlyFamily()
	if len(fam.Styles) < 2 {
		t.Fatalf("want >=2 styles from the collection, got %d", len(fam.Styles))
	}
	h := &fakeHelper{fontsDir: t.TempDir()}
	svc := NewInstallService(e.st, e.cat, e.act.mgr, nil, h.run, nil)
	res, err := svc.InstallForEveryone(fam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 0 {
		t.Errorf("helper called for a collection-only family: %+v", h.calls)
	}
	if len(res.Failed) != 1 || res.Failed[0].File != "pair.TTC" || res.Failed[0].Error != msg {
		t.Errorf("Failed = %+v", res.Failed)
	}

	// Mixed family: only the ttf reaches the helper.
	e2, dir2 := newEnv(t, "a.ttf")
	if err := os.WriteFile(filepath.Join(dir2, "pair.ttc"), ttcOf(ttf, ttf), 0o644); err != nil {
		t.Fatal(err)
	}
	e2.addRootAndScan(dir2)
	h2 := &fakeHelper{fontsDir: t.TempDir()}
	svc2 := NewInstallService(e2.st, e2.cat, e2.act.mgr, nil, h2.run, nil)
	res, err = svc2.InstallForEveryone(e2.onlyFamily().ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.calls) != 1 || len(h2.calls[0].Files) != 1 || filepath.Base(h2.calls[0].Files[0]) != "a.ttf" {
		t.Errorf("helper calls = %+v, want only a.ttf", h2.calls)
	}
	if len(res.Failed) != 1 || res.Failed[0].File != "pair.ttc" || res.Failed[0].Error != msg {
		t.Errorf("Failed = %+v", res.Failed)
	}
}
