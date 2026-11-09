// Package helperproto defines the JSON wire format used between
// skrifttype.exe (standard user) and font-helper.exe (elevated). The
// helper is invoked once per "install for everyone" / "uninstall for
// everyone" action via ShellExecuteEx with the runas verb. The main
// app passes a Request as JSON via a temp file, the helper writes a
// Result as JSON to a path the Request specifies, and the helper
// process exits with a status code. The protocol is one-shot — no
// streaming, no bidirectional channel.
package helperproto

// Op identifies the operation the helper should perform. New ops
// require updating both this package and the helper binary.
type Op string

const (
	// OpInstallSystem copies the listed files into %WINDIR%\Fonts and
	// adds an HKLM registry entry for each. Persists across reboots.
	OpInstallSystem Op = "install-system"

	// OpUninstallSystem removes the listed files from %WINDIR%\Fonts
	// and deletes their HKLM registry entries. The Files field of the
	// Request lists the absolute paths to remove (the helper does not
	// look anything up; the main app provides authoritative paths).
	OpUninstallSystem Op = "uninstall-system"
)

// Request is the input the main app passes to the helper as JSON.
type Request struct {
	// Op is which operation to perform. Required.
	Op Op `json:"op"`

	// Files is the list of absolute source paths the operation
	// applies to. Interpretation depends on Op (sources to copy for
	// install; targets to remove for uninstall). Required and
	// non-empty.
	Files []string `json:"files"`

	// ResultTo is the absolute path the helper MUST write its Result
	// to as JSON before exiting. The main app reads this file then
	// deletes it. Required.
	ResultTo string `json:"result_to"`
}

// Result is the output the helper writes to ResultTo as JSON before
// exiting.
type Result struct {
	// Op echoes the request's Op for sanity-checking.
	Op Op `json:"op"`

	// Success is true if every item in Items succeeded. If any
	// individual item failed, Success is false and Error summarises.
	Success bool `json:"success"`

	// Items records per-file outcomes in the same order as the
	// Request's Files. Length equals len(Request.Files).
	Items []ResultItem `json:"items"`

	// Error is a top-level error message when the helper could not
	// even attempt the operation (e.g. malformed Request).
	// Per-file errors live on individual ResultItems.
	Error string `json:"error,omitempty"`
}

// ResultItem records the outcome of one file within a Request.
type ResultItem struct {
	// SourcePath is the original path from Request.Files.
	SourcePath string `json:"source_path"`

	// InstalledAs is the destination path (e.g. C:\Windows\Fonts\X.ttf)
	// for install operations, or empty for uninstall.
	InstalledAs string `json:"installed_as,omitempty"`

	// RegistryKey is the value name written to (or removed from) the
	// HKLM fonts key, e.g. "Inter (TrueType)".
	RegistryKey string `json:"registry_key,omitempty"`

	// Error describes the per-file failure. Empty on success.
	Error string `json:"error,omitempty"`
}
