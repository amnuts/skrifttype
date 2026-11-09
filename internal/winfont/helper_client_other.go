//go:build !windows

package winfont

func runElevated(exe, arg string) (uint32, error) { return 0, ErrUnsupportedPlatform }
