package api

import "github.com/wailsapp/wails/v3/pkg/application"

// Registering events types them in the generated TS bindings and lets
// Emit validate payloads.
func init() {
	application.RegisterEvent[bool](EventLibraryScanning)
	application.RegisterEvent[ScanReportDTO](EventLibraryChanged)
}
