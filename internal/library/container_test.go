package library

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"golang.org/x/image/font/sfnt"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func familyName(t *testing.T, data []byte) string {
	t.Helper()
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	var buf sfnt.Buffer
	name, err := f.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		t.Fatalf("family name: %v", err)
	}
	return name
}

// buildTTC wraps the given single-face fonts in a ttcf collection. Each
// font is appended whole and its table offsets are shifted by its position.
func buildTTC(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	headerLen := 12 + 4*len(fonts)
	out := make([]byte, headerLen)
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

func TestSplitFacesSfnt(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	faces, c, err := SplitFaces(ttf)
	if err != nil {
		t.Fatal(err)
	}
	if c != ContainerSfnt || len(faces) != 1 || !bytes.Equal(faces[0], ttf) {
		t.Fatalf("got %d faces, container %q", len(faces), c)
	}
	if OutlineExt(faces[0]) != ".ttf" {
		t.Errorf("OutlineExt = %q", OutlineExt(faces[0]))
	}
}

func TestOutlineExt(t *testing.T) {
	if got := OutlineExt([]byte("OTTO....")); got != ".otf" {
		t.Errorf("OTTO: %q", got)
	}
	if got := OutlineExt(nil); got != ".ttf" {
		t.Errorf("nil: %q", got)
	}
}

func testWrapped(t *testing.T, fixture string, want Container) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	faces, c, err := SplitFaces(readFixture(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	if c != want || len(faces) != 1 {
		t.Fatalf("got %d faces, container %q", len(faces), c)
	}
	if got, exp := familyName(t, faces[0]), familyName(t, ttf); got != exp {
		t.Errorf("family %q, want %q", got, exp)
	}
}

func TestSplitFacesWOFF(t *testing.T)  { testWrapped(t, "Inter-Medium.woff", ContainerWOFF) }
func TestSplitFacesWOFF2(t *testing.T) { testWrapped(t, "Inter-Medium.woff2", ContainerWOFF2) }

func TestSplitFacesTTC(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	faces, c, err := SplitFaces(buildTTC(t, ttf, ttf))
	if err != nil {
		t.Fatal(err)
	}
	if c != ContainerTTC || len(faces) != 2 {
		t.Fatalf("got %d faces, container %q", len(faces), c)
	}
	want := familyName(t, ttf)
	for i, f := range faces {
		if got := familyName(t, f); got != want {
			t.Errorf("face %d family %q, want %q", i, got, want)
		}
	}
}

func TestExtractFaceOutOfRange(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	ttc := buildTTC(t, ttf, ttf)
	if _, err := ExtractFace(ttc, 1); err != nil {
		t.Fatalf("index 1: %v", err)
	}
	if _, err := ExtractFace(ttc, 2); err == nil {
		t.Error("index 2 should fail")
	}
	if _, err := ExtractFace(ttc, -1); err == nil {
		t.Error("index -1 should fail")
	}
}

func TestSplitFacesCorrupt(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	woff := readFixture(t, "Inter-Medium.woff")
	woff2 := readFixture(t, "Inter-Medium.woff2")

	hugeTTC := make([]byte, 12)
	copy(hugeTTC, "ttcf")
	binary.BigEndian.PutUint32(hugeTTC[4:], 0x00010000)
	binary.BigEndian.PutUint32(hugeTTC[8:], 1000000)

	badWOFF := append([]byte(nil), woff...)
	binary.BigEndian.PutUint32(badWOFF[44+4:], uint32(len(woff)+100)) // first table offset

	rnd := make([]byte, 4096)
	rand.New(rand.NewSource(1)).Read(rnd)

	cases := map[string][]byte{
		"empty":           nil,
		"three bytes":     {1, 2, 3},
		"huge ttc":        hugeTTC,
		"woff offset":     badWOFF,
		"random":          rnd,
		"truncated sfnt":  ttf[:100],
		"truncated woff":  woff[:200],
		"truncated woff2": woff2[:200],
		"truncated ttc":   buildTTC(t, ttf)[:14],
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := SplitFaces(data); err == nil {
				t.Error("expected error")
			}
		})
	}
}

// buildWOFF1 builds a one-table WOFF with the given directory fields.
func buildWOFF1(flavor string, comp, payload []byte, orig int) []byte {
	out := make([]byte, 44+20, 44+20+len(payload))
	copy(out, "wOFF")
	copy(out[4:], flavor)
	binary.BigEndian.PutUint32(out[8:], uint32(64+len(payload)))
	binary.BigEndian.PutUint16(out[12:], 1)
	copy(out[44:], "abcd")
	binary.BigEndian.PutUint32(out[48:], 64)
	binary.BigEndian.PutUint32(out[52:], uint32(len(comp)))
	binary.BigEndian.PutUint32(out[56:], uint32(orig))
	return append(out, payload...)
}

func TestSplitFacesWOFFZlibBomb(t *testing.T) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(make([]byte, 8<<20))
	zw.Close()
	// Declares 1 MiB but the stream inflates to 8 MiB (compressed is far
	// smaller than the declared size, so it takes the inflate path).
	data := buildWOFF1("\x00\x01\x00\x00", z.Bytes(), z.Bytes(), 1<<20)
	if _, _, err := SplitFaces(data); err == nil {
		t.Error("expected error")
	}
}

func TestSplitFacesWOFFBadFlavor(t *testing.T) {
	data := buildWOFF1("XXXX", []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4}, 4)
	if _, _, err := SplitFaces(data); err == nil {
		t.Error("expected error")
	}
}

func TestSplitFacesWOFF2Corrupt(t *testing.T) {
	woff2 := readFixture(t, "Inter-Medium.woff2")

	garbage := append([]byte(nil), woff2...)
	for i := len(garbage) - 2000; i < len(garbage); i++ {
		garbage[i] = byte(i*31 + 7)
	}
	huge := append([]byte(nil), woff2...)
	binary.BigEndian.PutUint32(huge[16:], 0xFFFFFFFF)

	// One cmap table declared as 1000 bytes whose brotli stream expands
	// to 64 MiB.
	var comp bytes.Buffer
	bw := brotli.NewWriter(&comp)
	bw.Write(make([]byte, 64<<20))
	bw.Close()
	dir := []byte{0x00, 0x87, 0x68} // flags (cmap, no transform), origLength 1000
	bomb := make([]byte, 48, 48+len(dir)+comp.Len())
	copy(bomb, "wOF2")
	copy(bomb[4:], "\x00\x01\x00\x00")
	binary.BigEndian.PutUint16(bomb[12:], 1)
	binary.BigEndian.PutUint32(bomb[16:], 1000)
	binary.BigEndian.PutUint32(bomb[20:], uint32(comp.Len()))
	bomb = append(bomb, dir...)
	bomb = append(bomb, comp.Bytes()...)
	binary.BigEndian.PutUint32(bomb[8:], uint32(len(bomb)))

	cases := map[string][]byte{
		"garbage payload":  garbage,
		"declared too big": huge,
		"brotli bomb":      bomb,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := SplitFaces(data); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestSplitFacesTTCOffsetPastEOF(t *testing.T) {
	ttc := buildTTC(t, readFixture(t, "Inter-Medium.ttf"))
	binary.BigEndian.PutUint32(ttc[12:], uint32(len(ttc)+10))
	if _, _, err := SplitFaces(ttc); err == nil {
		t.Error("expected error")
	}
}

// buildSharedTTC builds a two-face collection whose table directories
// point at one shared copy of the table data, as real collections do. The
// second face omits the last table of the original.
func buildSharedTTC(t *testing.T, ttf []byte) []byte {
	t.Helper()
	tables, err := readSfntTables(ttf, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := len(tables)
	dir := func(count int) int { return 12 + 16*count }
	f1, f2 := 12+8, 12+8+dir(n)
	dataStart := f2 + dir(n-1)
	offs := make([]int, n)
	var blob []byte
	for i, tb := range tables {
		offs[i] = dataStart + len(blob)
		blob = append(blob, tb.data...)
		for len(blob)%4 != 0 {
			blob = append(blob, 0)
		}
	}
	out := make([]byte, dataStart)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], 2)
	binary.BigEndian.PutUint32(out[12:], uint32(f1))
	binary.BigEndian.PutUint32(out[16:], uint32(f2))
	for face, base := range []int{f1, f2} {
		count := n - face
		copy(out[base:], ttf[:12])
		binary.BigEndian.PutUint16(out[base+4:], uint16(count))
		for i := 0; i < count; i++ {
			e := out[base+12+16*i:]
			copy(e, tables[i].tag[:])
			binary.BigEndian.PutUint32(e[4:], tables[i].checksum)
			binary.BigEndian.PutUint32(e[8:], uint32(offs[i]))
			binary.BigEndian.PutUint32(e[12:], uint32(len(tables[i].data)))
		}
	}
	return append(out, blob...)
}

func TestSplitFacesTTCSharedTables(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	orig, _ := readSfntTables(ttf, 0)
	faces, c, err := SplitFaces(buildSharedTTC(t, ttf))
	if err != nil {
		t.Fatal(err)
	}
	if c != ContainerTTC || len(faces) != 2 {
		t.Fatalf("got %d faces, container %q", len(faces), c)
	}
	for fi, face := range faces {
		tables, err := readSfntTables(face, 0)
		if err != nil {
			t.Fatalf("face %d: %v", fi, err)
		}
		if want := len(orig) - fi; len(tables) != want {
			t.Fatalf("face %d: %d tables, want %d", fi, len(tables), want)
		}
		for i := range tables {
			e := face[12+16*i:]
			if off := binary.BigEndian.Uint32(e[8:]); off%4 != 0 {
				t.Errorf("face %d table %q offset %d not aligned", fi, tables[i].tag, off)
			}
			if i > 0 && bytes.Compare(tables[i-1].tag[:], tables[i].tag[:]) >= 0 {
				t.Errorf("face %d tags not sorted at %d", fi, i)
			}
			want := findTable(ttf, string(tables[i].tag[:]))
			if !bytes.Equal(tables[i].data, want) {
				t.Errorf("face %d table %q differs from original", fi, tables[i].tag)
			}
		}
		if _, err := sfnt.Parse(face); err != nil {
			t.Errorf("face %d: %v", fi, err)
		}
	}
}

func TestSplitFacesRandomBodies(t *testing.T) {
	for _, magic := range []string{"\x00\x01\x00\x00", "OTTO", "ttcf", "wOFF", "wOF2"} {
		for seed := int64(1); seed <= 20; seed++ {
			rng := rand.New(rand.NewSource(seed))
			body := make([]byte, 64+rng.Intn(2048))
			rng.Read(body)
			copy(body, magic)
			if _, _, err := SplitFaces(body); err == nil {
				t.Errorf("magic %q seed %d: expected error", magic, seed)
			}
		}
	}
}

func TestExtractFaceTTCFastPath(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	ttc := buildTTC(t, ttf, ttf, ttf)
	faces, _, err := SplitFaces(ttc)
	if err != nil {
		t.Fatal(err)
	}
	for i := range faces {
		got, err := ExtractFace(ttc, i)
		if err != nil {
			t.Fatalf("face %d: %v", i, err)
		}
		if !bytes.Equal(got, faces[i]) {
			t.Errorf("face %d differs from SplitFaces", i)
		}
	}
	shared := buildSharedTTC(t, ttf)
	sf, _, err := SplitFaces(shared)
	if err != nil {
		t.Fatal(err)
	}
	for i := range sf {
		got, err := ExtractFace(shared, i)
		if err != nil || !bytes.Equal(got, sf[i]) {
			t.Errorf("shared face %d: err=%v equal=%v", i, err, bytes.Equal(got, sf[i]))
		}
	}
	for _, i := range []int{-1, 3, 1 << 30} {
		if _, err := ExtractFace(ttc, i); err == nil {
			t.Errorf("index %d: expected error", i)
		}
	}
}

func TestExtractFaceTTCDoesNotBuildOtherFaces(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	ttc := buildTTC(t, ttf, ttf)
	// Point face 1 past EOF: SplitFaces fails, face 0 must still extract.
	binary.BigEndian.PutUint32(ttc[16:], uint32(len(ttc)+10))
	if _, _, err := SplitFaces(ttc); err == nil {
		t.Fatal("SplitFaces should fail")
	}
	if _, err := ExtractFace(ttc, 0); err != nil {
		t.Errorf("face 0: %v", err)
	}
	if _, err := ExtractFace(ttc, 1); err == nil {
		t.Error("face 1: expected error")
	}
}

func TestExtractFaceTTCCorrupt(t *testing.T) {
	ttf := readFixture(t, "Inter-Medium.ttf")
	ttc := buildTTC(t, ttf)
	for _, n := range []int{4, 8, 11, 12, 14, 20, 40, 100} {
		if _, err := ExtractFace(ttc[:n], 0); err == nil {
			t.Errorf("truncated to %d: expected error", n)
		}
	}
	zero := append([]byte(nil), ttc...)
	binary.BigEndian.PutUint32(zero[8:], 0)
	huge := append([]byte(nil), ttc...)
	binary.BigEndian.PutUint32(huge[8:], 0xFFFFFFFF)
	for name, d := range map[string][]byte{"zero": zero, "huge": huge} {
		if _, err := ExtractFace(d, 0); err == nil {
			t.Errorf("%s face count: expected error", name)
		}
	}
}

func TestSplitFacesTTCTooManyFaces(t *testing.T) {
	data := make([]byte, 12+4*257)
	copy(data, "ttcf")
	binary.BigEndian.PutUint32(data[4:], 0x00010000)
	binary.BigEndian.PutUint32(data[8:], 257)
	_, _, err := SplitFaces(data)
	if err == nil || !strings.Contains(err.Error(), "too many faces") {
		t.Errorf("SplitFaces err = %v", err)
	}
	if _, err := ExtractFace(data, 0); err == nil || !strings.Contains(err.Error(), "too many faces") {
		t.Errorf("ExtractFace err = %v", err)
	}
}
