package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func parseFixture(t *testing.T, name string) []FontRecord {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := NewSfntParser().Parse(p)
	if err != nil {
		t.Fatalf("Parse %s: %v", name, err)
	}
	return recs
}

func TestParseTTCReturnsEachFace(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	path := writeTemp(t, "inter.ttc", buildTTC(t, ttf, ttf))
	recs, err := NewSfntParser().Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	for i, r := range recs {
		if r.FaceIndex != i {
			t.Errorf("record %d: FaceIndex = %d", i, r.FaceIndex)
		}
		if r.Format != "ttc" {
			t.Errorf("record %d: Format = %q, want ttc", i, r.Format)
		}
		if r.Family == "" || r.Family != recs[0].Family {
			t.Errorf("record %d: Family = %q, want %q", i, r.Family, recs[0].Family)
		}
		if len(r.Codepoints) == 0 {
			t.Errorf("record %d: no codepoints", i)
		}
	}
}

func TestParseWOFFAndWOFF2MatchTTF(t *testing.T) {
	want := parseFixture(t, "Inter-Medium.ttf")[0]
	for ext, format := range map[string]string{"woff": "woff", "woff2": "woff2"} {
		recs := parseFixture(t, "Inter-Medium."+ext)
		if len(recs) != 1 {
			t.Fatalf("%s: got %d records", ext, len(recs))
		}
		got := recs[0]
		if got.Format != format {
			t.Errorf("%s: Format = %q, want %q", ext, got.Format, format)
		}
		if got.Family != want.Family || got.WeightClass != want.WeightClass ||
			got.Classification != want.Classification {
			t.Errorf("%s: family/weight/class = %q/%d/%q, want %q/%d/%q", ext,
				got.Family, got.WeightClass, got.Classification,
				want.Family, want.WeightClass, want.Classification)
		}
		if !reflect.DeepEqual(got.Codepoints, want.Codepoints) {
			t.Errorf("%s: codepoint ranges differ from ttf", ext)
		}
	}
}

func TestParseCorruptContainerErrors(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	ttc := buildTTC(t, ttf)
	cases := map[string][]byte{
		"trunc.ttc":   ttc[:14],
		"bad.woff":    []byte("wOFF\x00\x01\x00\x00garbage"),
		"bad.woff2":   []byte("wOF2garbage"),
		"empty.ttf":   {},
		"garbage.ttc": []byte("ttcf\x00\x01\x00\x00\xff\xff\xff\xff"),
	}
	for name, data := range cases {
		if _, err := NewSfntParser().Parse(writeTemp(t, name, data)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// A TTC face whose name table is empty still parses: extractRecord swallows
// per-name lookup errors, so Family is "" (the scanner only falls back to
// the filename stem when Parse itself fails).
func TestParseTTCFaceWithBlankNameTable(t *testing.T) {
	ttf := append([]byte(nil), readFixture(t, "Inter-Medium.ttf")...)
	// Blank the name table by zeroing its record count and string offset.
	n := int(uint16(ttf[4])<<8 | uint16(ttf[5]))
	blanked := false
	for i := 0; i < n; i++ {
		e := ttf[12+i*16:]
		if string(e[:4]) != "name" {
			continue
		}
		off := int(uint32(e[8])<<24 | uint32(e[9])<<16 | uint32(e[10])<<8 | uint32(e[11]))
		for j := 2; j < 6; j++ { // count (2 bytes) and stringOffset (2 bytes)
			ttf[off+j] = 0
		}
		blanked = true
	}
	if !blanked {
		t.Fatal("no name table in fixture")
	}
	path := writeTemp(t, "blank.ttc", buildTTC(t, ttf))
	recs, err := NewSfntParser().Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(recs) != 1 || recs[0].Family != "" || recs[0].Format != "ttc" {
		t.Fatalf("got %+v", recs)
	}
}

func TestFormatFromExt(t *testing.T) {
	for in, want := range map[string]string{
		"a.ttf": "ttf", "a.OTF": "otf", "a.TtC": "ttc", "a.otc": "otc",
		"a.WOFF": "woff", "a.woff2": "woff2",
	} {
		if got := formatFromExt(in); got != want {
			t.Errorf("formatFromExt(%q) = %q, want %q", in, got, want)
		}
	}
}
