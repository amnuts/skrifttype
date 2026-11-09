//go:build !windows

package fonthelper

// newSystem returns nil: system-wide installs are Windows-only, and
// realMain reports that in the result.
func newSystem() system { return nil }
