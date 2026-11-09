package library_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/amnuts/skrifttype/internal/library"
)

func TestSfntParser_ReadsOpenTypeTables(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("testdata", "Inter-Medium.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := library.NewSfntParser().Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r := records[0]

	// Typographic names group Medium into the Inter family.
	if r.Family != "Inter" || r.Subfamily != "Medium" {
		t.Errorf("Family/Subfamily = %q/%q, want Inter/Medium", r.Family, r.Subfamily)
	}
	if r.WeightClass != 500 {
		t.Errorf("WeightClass = %d, want 500 (Medium)", r.WeightClass)
	}
	if r.WidthClass != 5 {
		t.Errorf("WidthClass = %d, want 5 (normal)", r.WidthClass)
	}
	if r.Italic {
		t.Error("Italic = true for an upright face")
	}
	if r.IsVariable {
		t.Error("IsVariable = true for a static instance")
	}
	for _, tag := range []string{"calt", "dlig", "tnum", "ss01"} {
		if !slices.Contains(r.GsubFeatures, tag) {
			t.Errorf("GsubFeatures %v missing %q", r.GsubFeatures, tag)
		}
	}
	if !slices.Contains(r.GposFeatures, "kern") {
		t.Errorf("GposFeatures %v missing kern", r.GposFeatures)
	}

	covers := func(cp int) bool {
		for _, rg := range r.Codepoints {
			if cp >= rg.Lo && cp <= rg.Hi {
				return true
			}
		}
		return false
	}
	for _, cp := range []int{'A', 'z', '0', 0x00E9 /* é */, 0x20AC /* € */} {
		if !covers(cp) {
			t.Errorf("cmap ranges do not cover U+%04X", cp)
		}
	}
	if covers(0x4E00) {
		t.Error("cmap claims CJK coverage for a Latin font")
	}
	for i := 1; i < len(r.Codepoints); i++ {
		if r.Codepoints[i].Lo <= r.Codepoints[i-1].Hi+1 {
			t.Fatalf("ranges not merged/sorted at %d: %v %v", i, r.Codepoints[i-1], r.Codepoints[i])
		}
	}
}
