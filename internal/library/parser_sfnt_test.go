package library_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnuts/skrifttype/internal/library"
)

func TestSfntParser_ParsesInterFixture(t *testing.T) {
	p := library.NewSfntParser()
	path, err := filepath.Abs(filepath.Join("testdata", "Inter-Medium.ttf"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	records, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 record for a single-face TTF, got %d", len(records))
	}
	r := records[0]

	if !strings.Contains(strings.ToLower(r.Family), "inter") {
		t.Errorf("Family: got %q, expected to contain 'inter'", r.Family)
	}
	// The weight belongs to the subfamily (typographic names), not the
	// family, so every Inter weight groups into one family.
	if !strings.Contains(strings.ToLower(r.Subfamily), "medium") {
		t.Errorf("Subfamily: got %q, expected to contain 'medium'", r.Subfamily)
	}
	if r.FullName == "" {
		t.Error("FullName: expected non-empty")
	}
	if r.PostscriptName == "" {
		t.Error("PostscriptName: expected non-empty")
	}
	if r.Format != "ttf" {
		t.Errorf("Format: got %q, want ttf", r.Format)
	}
	if r.FaceIndex != 0 {
		t.Errorf("FaceIndex: got %d, want 0", r.FaceIndex)
	}
	if r.ParseError != "" {
		t.Errorf("ParseError: got %q, expected empty for a valid font", r.ParseError)
	}
}

func TestSfntParser_MissingFile(t *testing.T) {
	p := library.NewSfntParser()
	_, err := p.Parse(`Z:\does\not\exist.ttf`)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestSfntParser_NotAFont(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "fake.ttf")
	if err := os.WriteFile(bad, []byte("this is not a font"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p := library.NewSfntParser()
	_, err2 := p.Parse(bad)
	if err2 == nil {
		t.Fatal("expected error for non-font content")
	}
}
