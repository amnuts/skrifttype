package helperproto_test

import (
	"encoding/json"
	"testing"

	"github.com/amnuts/skrifttype/pkg/helperproto"
)

func TestRequest_RoundTripJSON(t *testing.T) {
	in := helperproto.Request{
		Op:       helperproto.OpInstallSystem,
		Files:    []string{`C:\fonts\one.ttf`, `C:\fonts\two.ttf`},
		ResultTo: `C:\Users\test\AppData\Local\Temp\helper-result-1234.json`,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out helperproto.Request
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if out.Op != in.Op {
		t.Errorf("Op: got %q, want %q", out.Op, in.Op)
	}
	if len(out.Files) != len(in.Files) {
		t.Fatalf("Files len: got %d, want %d", len(out.Files), len(in.Files))
	}
	for i := range in.Files {
		if out.Files[i] != in.Files[i] {
			t.Errorf("Files[%d]: got %q, want %q", i, out.Files[i], in.Files[i])
		}
	}
	if out.ResultTo != in.ResultTo {
		t.Errorf("ResultTo: got %q, want %q", out.ResultTo, in.ResultTo)
	}
}

func TestResult_RoundTripJSON_Success(t *testing.T) {
	in := helperproto.Result{
		Op:      helperproto.OpInstallSystem,
		Success: true,
		Items: []helperproto.ResultItem{
			{
				SourcePath:  `C:\fonts\one.ttf`,
				InstalledAs: `C:\Windows\Fonts\one.ttf`,
				RegistryKey: `One (TrueType)`,
			},
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out helperproto.Result
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if out.Op != in.Op {
		t.Errorf("Op: got %q, want %q", out.Op, in.Op)
	}
	if out.Success != in.Success {
		t.Errorf("Success: got %v, want %v", out.Success, in.Success)
	}
	if len(out.Items) != len(in.Items) {
		t.Fatalf("Items len: got %d, want %d", len(out.Items), len(in.Items))
	}
	if out.Items[0] != in.Items[0] {
		t.Errorf("Items[0]: got %+v, want %+v", out.Items[0], in.Items[0])
	}
	if out.Error != "" {
		t.Errorf("Error: expected empty on success, got %q", out.Error)
	}
}

func TestResult_RoundTripJSON_Failure(t *testing.T) {
	in := helperproto.Result{
		Op:      helperproto.OpInstallSystem,
		Success: false,
		Items: []helperproto.ResultItem{
			{
				SourcePath: `C:\fonts\bad.ttf`,
				Error:      "access denied",
			},
		},
		Error: "1 of 1 files failed",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Confirm the JSON omits empty strings for InstalledAs and
	// RegistryKey on a failed item.
	got := string(data)
	wantInstalledAs := `"installed_as":""`
	if containsSubstring(got, wantInstalledAs) {
		t.Errorf("expected omitempty on InstalledAs; JSON contained %q", wantInstalledAs)
	}
	wantRegistryKey := `"registry_key":""`
	if containsSubstring(got, wantRegistryKey) {
		t.Errorf("expected omitempty on RegistryKey; JSON contained %q", wantRegistryKey)
	}

	var out helperproto.Result
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Success {
		t.Error("Success: expected false")
	}
	if out.Error != in.Error {
		t.Errorf("Error: got %q, want %q", out.Error, in.Error)
	}
	if out.Items[0].Error != in.Items[0].Error {
		t.Errorf("Items[0].Error: got %q, want %q", out.Items[0].Error, in.Items[0].Error)
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
