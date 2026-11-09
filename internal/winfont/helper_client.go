package winfont

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amnuts/skrifttype/pkg/helperproto"
)

// HelperExeName is the elevated helper binary shipped next to skrifttype.exe.
const HelperExeName = "font-helper.exe"

// ErrElevationDeclined is returned when the user answers "No" to the UAC
// prompt. Nothing was changed.
var ErrElevationDeclined = errors.New("permission to install for all users was declined")

// HelperPath returns the helper's expected location (next to the running
// executable) and whether it exists.
func HelperPath() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	p := filepath.Join(filepath.Dir(exe), HelperExeName)
	info, err := os.Stat(p)
	return p, err == nil && info.Mode().IsRegular()
}

// RunHelper performs a system-wide operation via the elevated helper:
// it writes the request into a fresh private temp directory, launches
// helperExe through UAC, waits for it, and returns its result. The
// caller broadcasts WM_FONTCHANGE afterwards (the helper never does).
func RunHelper(helperExe string, op helperproto.Op, files []string) (helperproto.Result, error) {
	dir, err := os.MkdirTemp("", "skrifttype-helper-")
	if err != nil {
		return helperproto.Result{}, err
	}
	defer os.RemoveAll(dir)

	reqPath := filepath.Join(dir, "request.json")
	resPath := filepath.Join(dir, "result.json")
	data, err := json.Marshal(helperproto.Request{Op: op, Files: files, ResultTo: resPath})
	if err != nil {
		return helperproto.Result{}, err
	}
	if err := os.WriteFile(reqPath, data, 0o600); err != nil {
		return helperproto.Result{}, err
	}

	code, err := runElevated(helperExe, reqPath)
	if err != nil {
		return helperproto.Result{}, err
	}

	var res helperproto.Result
	out, readErr := os.ReadFile(resPath)
	if readErr != nil {
		return res, fmt.Errorf("font helper exited with code %d and no result: %w", code, readErr)
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return res, fmt.Errorf("font helper result: %w", err)
	}
	return res, nil
}
