//go:build windows

package winfont

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
	swHide                = 0
)

// shellExecuteInfo mirrors SHELLEXECUTEINFOW. Go's natural field
// alignment matches the Win32 layout on amd64 and arm64.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

// runElevated launches exe with the "runas" verb (UAC prompt), waits for
// it to exit, and returns its exit code.
func runElevated(exe, arg string) (uint32, error) { return shellExecWait("runas", exe, arg) }

// shellExecWait runs exe via ShellExecuteExW with the given verb, waits
// for it to exit, and returns its exit code.
func shellExecWait(verbName, exe, arg string) (uint32, error) {
	verb, err := windows.UTF16PtrFromString(verbName)
	if err != nil {
		return 0, err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(syscall.EscapeArg(arg))
	if err != nil {
		return 0, err
	}
	sei := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        swHide,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	if r, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei))); r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrElevationDeclined
		}
		return 0, fmt.Errorf("launching font helper: %w", callErr)
	}
	if sei.hProcess == 0 {
		return 0, errors.New("launching font helper: no process handle")
	}
	defer windows.CloseHandle(sei.hProcess)

	if _, err := windows.WaitForSingleObject(sei.hProcess, windows.INFINITE); err != nil {
		return 0, fmt.Errorf("waiting for font helper: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(sei.hProcess, &code); err != nil {
		return 0, fmt.Errorf("font helper exit code: %w", err)
	}
	return code, nil
}
