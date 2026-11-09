//go:build windows

package fonthelper

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const fontsKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`

var (
	gdi32                     = windows.NewLazySystemDLL("gdi32.dll")
	procAddFontResourceExW    = gdi32.NewProc("AddFontResourceExW")
	procRemoveFontResourceExW = gdi32.NewProc("RemoveFontResourceExW")
)

type windowsSystem struct{ fontsDir string }

func newSystem() system {
	dir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil
	}
	return &windowsSystem{fontsDir: filepath.Join(dir, "Fonts")}
}

func (s *windowsSystem) FontsDir() string { return s.fontsDir }

func (s *windowsSystem) FontValues() (map[string]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, fontsKey, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		if v, _, err := k.GetStringValue(n); err == nil {
			out[n] = v
		}
	}
	return out, nil
}

func (s *windowsSystem) SetFontValue(name, data string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, fontsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, data)
}

func (s *windowsSystem) DeleteFontValue(name string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, fontsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.DeleteValue(name)
}

func (s *windowsSystem) AddFontResource(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if r, _, callErr := procAddFontResourceExW.Call(uintptr(unsafe.Pointer(p)), 0, 0); r == 0 {
		return fmt.Errorf("AddFontResourceExW: %w", callErr)
	}
	return nil
}

func (s *windowsSystem) RemoveFontResource(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if r, _, callErr := procRemoveFontResourceExW.Call(uintptr(unsafe.Pointer(p)), 0, 0); r == 0 {
		return fmt.Errorf("RemoveFontResourceExW: %w", callErr)
	}
	return nil
}
