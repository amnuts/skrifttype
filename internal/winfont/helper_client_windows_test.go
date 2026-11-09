//go:build windows

package winfont

import (
	"os"
	"path/filepath"
	"testing"
)

// The real helper needs the "runas" verb, which shows a UAC prompt, so
// the ShellExecuteEx plumbing (struct layout, process handle, wait, exit
// code) is exercised with the plain "open" verb on a harmless tool.
func TestShellExecWait_ReturnsExitCode(t *testing.T) {
	where := filepath.Join(os.Getenv("WINDIR"), "System32", "where.exe")
	if _, err := os.Stat(where); err != nil {
		t.Skip("where.exe not available")
	}
	code, err := shellExecWait("open", where, "skrifttype-no-such-program-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (not found)", code)
	}
	code, err = shellExecWait("open", where, "where.exe")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestShellExecWait_MissingExecutable(t *testing.T) {
	if _, err := shellExecWait("open", filepath.Join(t.TempDir(), "nope.exe"), ""); err == nil {
		t.Error("expected an error launching a missing executable")
	}
}
