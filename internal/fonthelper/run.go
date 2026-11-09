// Package fonthelper implements the font-helper command, which performs system-wide font installs for skrifttype.
//
// It is a separate, tiny binary whose manifest requires administrator
// rights, so skrifttype.exe itself never runs elevated (spec §7). The main
// app launches it through UAC with one argument: the path to a
// helperproto.Request JSON file. The helper writes a helperproto.Result
// to the request's ResultTo and exits:
//
//	0  every file succeeded
//	1  at least one file failed (see Result.Items)
//	2  the request itself was unusable (nothing was attempted)
//
// The helper never broadcasts WM_FONTCHANGE; the main app does that once
// after reading the result.
package fonthelper

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/amnuts/skrifttype/pkg/helperproto"
)

const maxRequestSize = 1 << 20

// Main runs the helper with the process arguments and returns its exit code.
func Main() int {
	return realMain(os.Args[1:], newSystem())
}

func realMain(args []string, sys system) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: font-helper <request.json>")
		return 2
	}
	reqPath := args[0]
	req, err := readRequest(reqPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "font-helper:", err)
		return 2
	}
	if err := validateResultPath(reqPath, req.ResultTo, os.Getenv("WINDIR")); err != nil {
		fmt.Fprintln(os.Stderr, "font-helper: bad result path:", err)
		return 2
	}

	var res helperproto.Result
	if sys == nil {
		res = helperproto.Result{Op: req.Op, Error: "system-wide installs are only supported on Windows"}
	} else {
		res = run(req, sys)
	}

	out, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintln(os.Stderr, "font-helper: encoding result:", err)
		return 2
	}
	if err := writeExclusive(req.ResultTo, out); err != nil {
		fmt.Fprintln(os.Stderr, "font-helper: writing result:", err)
		return 2
	}
	if !res.Success {
		return 1
	}
	return 0
}

func readRequest(path string) (helperproto.Request, error) {
	var req helperproto.Request
	fh, err := os.Open(path)
	if err != nil {
		return req, err
	}
	defer fh.Close()
	data, err := io.ReadAll(io.LimitReader(fh, maxRequestSize+1))
	if err != nil {
		return req, err
	}
	if len(data) > maxRequestSize {
		return req, fmt.Errorf("request is larger than %d bytes", maxRequestSize)
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("decoding request: %w", err)
	}
	return req, nil
}
