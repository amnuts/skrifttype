package convert

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "library", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustParse(t *testing.T, path string) *sfnt.Font {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := sfnt.Parse(b)
	if err != nil {
		t.Fatalf("sfnt.Parse(%s): %v", path, err)
	}
	return f
}

func psName(t *testing.T, path string) string {
	t.Helper()
	var buf sfnt.Buffer
	n, err := mustParse(t, path).Name(&buf, sfnt.NameIDPostScript)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// ttcOf wraps single-face fonts in a ttcf collection.
func ttcOf(fonts ...[]byte) []byte {
	out := make([]byte, 12+4*len(fonts))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(fonts)))
	for i, f := range fonts {
		base := len(out)
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(base))
		c := append([]byte(nil), f...)
		n := int(binary.BigEndian.Uint16(c[4:6]))
		for j := 0; j < n; j++ {
			p := 12 + j*16 + 8
			binary.BigEndian.PutUint32(c[p:], binary.BigEndian.Uint32(c[p:])+uint32(base))
		}
		out = append(out, c...)
	}
	return out
}

// patchPSName flips the case of the last byte of every PostScript name
// (name ID 6) string in place, so offsets stay valid.
func patchPSName(t *testing.T, ttf []byte) []byte {
	t.Helper()
	out := append([]byte(nil), ttf...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) != "name" {
			continue
		}
		tab := int(binary.BigEndian.Uint32(out[rec+8:]))
		count := int(binary.BigEndian.Uint16(out[tab+2:]))
		strOff := tab + int(binary.BigEndian.Uint16(out[tab+4:]))
		for j := 0; j < count; j++ {
			r := tab + 6 + j*12
			if binary.BigEndian.Uint16(out[r+6:]) != 6 {
				continue
			}
			length := int(binary.BigEndian.Uint16(out[r+8:]))
			off := strOff + int(binary.BigEndian.Uint16(out[r+10:]))
			out[off+length-1] ^= 0x20
		}
		return out
	}
	t.Fatal("no name table")
	return nil
}

func TestConvertWOFF2ToTTF(t *testing.T) {
	src := t.TempDir()
	orig := fixture(t, "Inter-Medium.woff2")
	p := writeTemp(t, src, "Inter-Medium.woff2", orig)
	out := filepath.Join(t.TempDir(), "out")

	res, err := Faces(p, nil, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Err != "" {
		t.Fatalf("results = %+v", res)
	}
	if filepath.Ext(res[0].Path) != ".ttf" {
		t.Errorf("ext = %q, want .ttf", filepath.Ext(res[0].Path))
	}
	if filepath.Dir(res[0].Path) != out {
		t.Errorf("written to %q, want inside %q", res[0].Path, out)
	}
	mustParse(t, res[0].Path)
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, orig) {
		t.Error("source file was modified")
	}
}

func TestConvertWOFFToTTF(t *testing.T) {
	p := writeTemp(t, t.TempDir(), "x.woff", fixture(t, "Inter-Medium.woff"))
	res, err := Faces(p, nil, t.TempDir())
	if err != nil || len(res) != 1 || res[0].Err != "" || filepath.Ext(res[0].Path) != ".ttf" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	mustParse(t, res[0].Path)
}

func TestConvertTTCSplitsAllFaces(t *testing.T) {
	ttf := fixture(t, "Inter-Medium.ttf")
	p := writeTemp(t, t.TempDir(), "pair.ttc", ttcOf(ttf, patchPSName(t, ttf)))
	out := t.TempDir()

	res, err := Faces(p, nil, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("results = %+v", res)
	}
	names := map[string]bool{}
	for i, r := range res {
		if r.Err != "" || r.FaceIndex != i || filepath.Ext(r.Path) != ".ttf" {
			t.Fatalf("result %d = %+v", i, r)
		}
		names[psName(t, r.Path)] = true
	}
	if len(names) != 2 {
		t.Errorf("faces not distinguishable: %v", names)
	}

	// A single requested face only writes that face.
	out2 := t.TempDir()
	res, err = Faces(p, []int{1}, out2)
	if err != nil || len(res) != 1 || res[0].FaceIndex != 1 || res[0].Err != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	entries, _ := os.ReadDir(out2)
	if len(entries) != 1 {
		t.Errorf("%d files written, want 1", len(entries))
	}
}

func TestConvertOTTOUsesOTFExt(t *testing.T) {
	ttf := append([]byte(nil), fixture(t, "Inter-Medium.ttf")...)
	copy(ttf, "OTTO")
	p := writeTemp(t, t.TempDir(), "cff.ttc", ttcOf(ttf))
	res, err := Faces(p, nil, t.TempDir())
	if err != nil || len(res) != 1 || res[0].Err != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if filepath.Ext(res[0].Path) != ".otf" {
		t.Errorf("ext = %q, want .otf", filepath.Ext(res[0].Path))
	}
}

func TestConvertNeverOverwrites(t *testing.T) {
	p := writeTemp(t, t.TempDir(), "a.woff2", fixture(t, "Inter-Medium.woff2"))
	out := t.TempDir()
	first, err := Faces(p, nil, out)
	if err != nil || first[0].Err != "" {
		t.Fatalf("first: %+v %v", first, err)
	}
	// Replace the output with sentinel content, then convert again.
	sentinel := []byte("precious")
	if err := os.WriteFile(first[0].Path, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Faces(p, nil, out)
	if err != nil || second[0].Err != "" {
		t.Fatalf("second: %+v %v", second, err)
	}
	if second[0].Path == first[0].Path {
		t.Fatal("second conversion reused the existing path")
	}
	if !strings.HasSuffix(second[0].Path, "-2.ttf") {
		t.Errorf("path = %q, want -2 suffix", second[0].Path)
	}
	got, _ := os.ReadFile(first[0].Path)
	if !bytes.Equal(got, sentinel) {
		t.Error("existing file was overwritten")
	}
	third, _ := Faces(p, nil, out)
	if !strings.HasSuffix(third[0].Path, "-3.ttf") {
		t.Errorf("path = %q, want -3 suffix", third[0].Path)
	}
}

func TestConvertSameNameFacesGetDistinctNames(t *testing.T) {
	ttf := fixture(t, "Inter-Medium.ttf")
	p := writeTemp(t, t.TempDir(), "dup.ttc", ttcOf(ttf, ttf))
	res, err := Faces(p, nil, t.TempDir())
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res[0].Err != "" || res[1].Err != "" || res[0].Path == res[1].Path {
		t.Errorf("results = %+v", res)
	}
}

func TestConvertUnwritableDirReportsPerFaceError(t *testing.T) {
	ttf := fixture(t, "Inter-Medium.ttf")
	p := writeTemp(t, t.TempDir(), "pair.ttc", ttcOf(ttf, patchPSName(t, ttf)))
	blocker := writeTemp(t, t.TempDir(), "file", []byte("x"))
	res, err := Faces(p, nil, filepath.Join(blocker, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("results = %+v", res)
	}
	for _, r := range res {
		if r.Err == "" || r.Path != "" {
			t.Errorf("result = %+v, want error and no path", r)
		}
	}
}

func TestConvertBadFaceIndexIsPerFaceError(t *testing.T) {
	p := writeTemp(t, t.TempDir(), "a.woff", fixture(t, "Inter-Medium.woff"))
	res, err := Faces(p, []int{0, 5}, t.TempDir())
	if err != nil || len(res) != 2 || res[0].Err != "" || res[1].Err == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestConvertSanitisesFileNames(t *testing.T) {
	cases := map[string]string{
		`a<b>c:d"e/f\g|h?i*j`: "a_b_c_d_e_f_g_h_i_j",
		"trail. . ":           "trail",
		"ctl\x01\x1fx":        "ctl__x",
		"CON":                 "_CON",
		"nul.txt":             "_nul.txt",
		"COM1":                "_COM1",
		"COM10":               "COM10",
		"???":                 "",
		"":                    "",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitize(strings.Repeat("a", 500)); len(got) != maxNameLen {
		t.Errorf("long name len = %d, want %d", len(got), maxNameLen)
	}
}

func TestConvertNameFallsBackToSourceStem(t *testing.T) {
	if got := baseName([]byte("not a font"), "my font"); got != "my font" {
		t.Errorf("baseName = %q", got)
	}
}
