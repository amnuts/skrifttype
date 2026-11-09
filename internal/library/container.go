package library

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/font"
)

// Container identifies how a font file wraps its sfnt data.
type Container string

const (
	ContainerSfnt  Container = "sfnt"
	ContainerTTC   Container = "ttc"
	ContainerWOFF  Container = "woff"
	ContainerWOFF2 Container = "woff2"
)

// maxDecodedSize caps the size of any decoded or rebuilt font so a
// corrupt or hostile file cannot exhaust memory.
const maxDecodedSize = 256 << 20

// The WOFF2 decoder rejects fonts above its own package-level limit
// (30 MiB by default). Raise it to the same cap used for WOFF and TTC so a
// large font does not load in one container and fail in another.
func init() {
	font.MaxMemory = maxDecodedSize
}

// sfntTable is one table of a font being reassembled.
type sfntTable struct {
	tag      [4]byte
	checksum uint32
	data     []byte
}

// SplitFaces normalises a font file into standalone single-face sfnt byte
// slices. Plain sfnt files yield one face (the input itself), TTC files
// yield one face per member, and WOFF/WOFF2 files are decoded first.
// Unknown magic, truncated data, out-of-range offsets and zero faces are
// errors. It never panics on malformed input.
func SplitFaces(data []byte) (faces [][]byte, c Container, err error) {
	defer func() {
		if r := recover(); r != nil {
			faces, c, err = nil, "", fmt.Errorf("font container: malformed data: %v", r)
		}
	}()
	if len(data) < 4 {
		return nil, "", errors.New("font container: file too short")
	}
	switch string(data[:4]) {
	case "wOFF":
		sf, err := decodeWOFF(data)
		if err != nil {
			return nil, "", err
		}
		return [][]byte{sf}, ContainerWOFF, nil
	case "wOF2":
		faces, err := decodeWOFF2(data)
		if err != nil {
			return nil, "", err
		}
		return faces, ContainerWOFF2, nil
	case "ttcf":
		faces, err := splitTTC(data)
		if err != nil {
			return nil, "", err
		}
		return faces, ContainerTTC, nil
	}
	if !isSfntMagic(data) {
		return nil, "", errors.New("font container: unrecognised font format")
	}
	if _, err := readSfntTables(data, 0); err != nil {
		return nil, "", err
	}
	return [][]byte{data}, ContainerSfnt, nil
}

// ExtractFace returns the standalone sfnt bytes of one face of a font
// file. It is an error if faceIndex is out of range. For a TTC only the
// requested face is rebuilt.
func ExtractFace(data []byte, faceIndex int) (face []byte, err error) {
	if len(data) >= 4 && string(data[:4]) == "ttcf" {
		defer func() {
			if r := recover(); r != nil {
				face, err = nil, fmt.Errorf("font container: malformed data: %v", r)
			}
		}()
		num, err := ttcFaceCount(data)
		if err != nil {
			return nil, err
		}
		if faceIndex < 0 || faceIndex >= num {
			return nil, fmt.Errorf("font container: face index %d out of range (have %d)", faceIndex, num)
		}
		return buildTTCFace(data, faceIndex)
	}
	faces, _, err := SplitFaces(data)
	if err != nil {
		return nil, err
	}
	if faceIndex < 0 || faceIndex >= len(faces) {
		return nil, fmt.Errorf("font container: face index %d out of range (have %d)", faceIndex, len(faces))
	}
	return faces[faceIndex], nil
}

// OutlineExt returns ".otf" for CFF-flavoured sfnt data (tag OTTO) and
// ".ttf" otherwise.
func OutlineExt(face []byte) string {
	if len(face) >= 4 && string(face[:4]) == "OTTO" {
		return ".otf"
	}
	return ".ttf"
}

func isSfntMagic(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	switch string(b[:4]) {
	case "\x00\x01\x00\x00", "OTTO", "true", "typ1":
		return true
	}
	return false
}

// readSfntTables reads the table directory of the sfnt starting at base
// and returns its tables, erroring if any table lies outside data.
func readSfntTables(data []byte, base int) ([]sfntTable, error) {
	if base < 0 || base > len(data) || len(data)-base < 12 {
		return nil, errors.New("font container: truncated sfnt header")
	}
	if !isSfntMagic(data[base:]) {
		return nil, errors.New("font container: bad sfnt signature")
	}
	n := int(binary.BigEndian.Uint16(data[base+4:]))
	if n == 0 {
		return nil, errors.New("font container: sfnt has no tables")
	}
	dirEnd := base + 12 + n*16
	if dirEnd > len(data) {
		return nil, errors.New("font container: truncated sfnt table directory")
	}
	tables := make([]sfntTable, n)
	for i := range tables {
		e := data[base+12+i*16:]
		off := int(binary.BigEndian.Uint32(e[8:]))
		length := int(binary.BigEndian.Uint32(e[12:]))
		if off < 0 || length < 0 || off > len(data) || length > len(data)-off {
			return nil, errors.New("font container: table out of range")
		}
		copy(tables[i].tag[:], e[:4])
		tables[i].checksum = binary.BigEndian.Uint32(e[4:])
		tables[i].data = data[off : off+length]
	}
	return tables, nil
}

// buildSfnt assembles a standalone sfnt from tables, sorted by tag, with
// 4-byte padded table data and recomputed directory offsets. Table
// checksums are kept as given.
func buildSfnt(magic []byte, tables []sfntTable) ([]byte, error) {
	sorted := append([]sfntTable(nil), tables...)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i].tag[:], sorted[j].tag[:]) < 0
	})
	n := len(sorted)
	size := 12 + 16*n
	for _, t := range sorted {
		size += (len(t.data) + 3) &^ 3
		if size > maxDecodedSize {
			return nil, errors.New("font container: decoded font too large")
		}
	}
	out := make([]byte, 12+16*n, size)
	copy(out, magic)
	binary.BigEndian.PutUint16(out[4:], uint16(n))
	pow, log := 1, 0
	for pow*2 <= n {
		pow *= 2
		log++
	}
	binary.BigEndian.PutUint16(out[6:], uint16(pow*16))
	binary.BigEndian.PutUint16(out[8:], uint16(log))
	binary.BigEndian.PutUint16(out[10:], uint16(n*16-pow*16))
	for i, t := range sorted {
		e := out[12+i*16:]
		copy(e, t.tag[:])
		binary.BigEndian.PutUint32(e[4:], t.checksum)
		binary.BigEndian.PutUint32(e[8:], uint32(len(out)))
		binary.BigEndian.PutUint32(e[12:], uint32(len(t.data)))
		out = append(out, t.data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out, nil
}

// maxTTCFaces caps the number of faces accepted from one collection.
const maxTTCFaces = 256

// ttcFaceCount validates a TTC header and returns its face count.
func ttcFaceCount(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, errors.New("font container: truncated ttc header")
	}
	num := int(binary.BigEndian.Uint32(data[8:]))
	if num <= 0 {
		return 0, errors.New("font container: ttc has no faces")
	}
	if num > maxTTCFaces {
		return 0, errors.New("font container: ttc has too many faces")
	}
	if num > (len(data)-12)/4 {
		return 0, errors.New("font container: ttc face count exceeds file size")
	}
	return num, nil
}

// buildTTCFace rebuilds face i of a validated TTC as a standalone sfnt.
func buildTTCFace(data []byte, i int) ([]byte, error) {
	off := int(binary.BigEndian.Uint32(data[12+4*i:]))
	tables, err := readSfntTables(data, off)
	if err != nil {
		return nil, fmt.Errorf("ttc face %d: %w", i, err)
	}
	return buildSfnt(data[off:off+4], tables)
}

// splitTTC extracts every member of a TrueType Collection as a
// standalone sfnt.
func splitTTC(data []byte) ([][]byte, error) {
	num, err := ttcFaceCount(data)
	if err != nil {
		return nil, err
	}
	faces := make([][]byte, 0, num)
	total := 0
	for i := 0; i < num; i++ {
		face, err := buildTTCFace(data, i)
		if err != nil {
			return nil, err
		}
		if total += len(face); total > maxDecodedSize {
			return nil, errors.New("font container: decoded font too large")
		}
		faces = append(faces, face)
	}
	return faces, nil
}

// decodeWOFF converts a WOFF 1.0 file to a standalone sfnt.
func decodeWOFF(data []byte) ([]byte, error) {
	if len(data) < 44 {
		return nil, errors.New("font container: truncated woff header")
	}
	if !isSfntMagic(data[4:]) {
		return nil, errors.New("font container: woff has bad flavor")
	}
	n := int(binary.BigEndian.Uint16(data[12:]))
	if n == 0 {
		return nil, errors.New("font container: woff has no tables")
	}
	if 44+n*20 > len(data) {
		return nil, errors.New("font container: truncated woff table directory")
	}
	tables := make([]sfntTable, n)
	total := 0
	for i := range tables {
		e := data[44+i*20:]
		off := int(binary.BigEndian.Uint32(e[4:]))
		comp := int(binary.BigEndian.Uint32(e[8:]))
		orig := int(binary.BigEndian.Uint32(e[12:]))
		if off < 0 || comp < 0 || orig < 0 || off > len(data) || comp > len(data)-off {
			return nil, errors.New("font container: woff table out of range")
		}
		if total += orig; total > maxDecodedSize || orig > maxDecodedSize {
			return nil, errors.New("font container: decoded font too large")
		}
		raw := data[off : off+comp]
		if comp < orig {
			zr, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				return nil, fmt.Errorf("font container: woff table: %w", err)
			}
			raw, err = io.ReadAll(io.LimitReader(zr, int64(orig)+1))
			zr.Close()
			if err != nil {
				return nil, fmt.Errorf("font container: woff table: %w", err)
			}
			if len(raw) != orig {
				return nil, errors.New("font container: woff table length mismatch")
			}
		} else if comp > orig {
			return nil, errors.New("font container: woff table length mismatch")
		}
		copy(tables[i].tag[:], e[:4])
		tables[i].checksum = binary.BigEndian.Uint32(e[16:])
		tables[i].data = raw
	}
	return buildSfnt(data[4:8], tables)
}

// decodeWOFF2 converts a WOFF2 file to standalone sfnt faces, converting
// decoder panics into errors.
func decodeWOFF2(data []byte) (faces [][]byte, err error) {
	if len(data) < 48 {
		return nil, errors.New("font container: truncated woff2 header")
	}
	if binary.BigEndian.Uint32(data[16:]) > maxDecodedSize {
		return nil, errors.New("font container: decoded font too large")
	}
	if err := checkWOFF2Stream(data); err != nil {
		return nil, err
	}
	var sf []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("font container: woff2 decode failed: %v", r)
			}
		}()
		sf, err = font.ParseWOFF2(data)
	}()
	if err != nil {
		return nil, fmt.Errorf("font container: woff2: %w", err)
	}
	if len(sf) > maxDecodedSize {
		return nil, errors.New("font container: decoded font too large")
	}
	if len(sf) >= 4 && string(sf[:4]) == "ttcf" {
		return splitTTC(sf)
	}
	if !isSfntMagic(sf) {
		return nil, errors.New("font container: woff2 decoded to non-sfnt data")
	}
	if _, err := readSfntTables(sf, 0); err != nil {
		return nil, err
	}
	return [][]byte{sf}, nil
}

// woff2KnownTags holds the indices of the WOFF2 known-table tags that
// affect whether a table directory entry carries a transform length.
const (
	woff2TagHmtx = 3
	woff2TagGlyf = 10
	woff2TagLoca = 11
)

// checkWOFF2Stream bounds the real output of the brotli stream before the
// decoder library sees it. The library only checks declared sizes and then
// copies the stream into memory unbounded, so a small stream could expand
// without limit. Here the table directory is walked to find the stream and
// the declared uncompressed size, and the stream is decoded into a counter
// that fails as soon as it exceeds that size (itself capped at
// maxDecodedSize). Any mismatch other than overrun is left to the library.
func checkWOFF2Stream(data []byte) error {
	numTables := int(binary.BigEndian.Uint16(data[12:]))
	compLen := int64(binary.BigEndian.Uint32(data[20:]))
	pos := 48
	var declared int64
	for i := 0; i < numTables; i++ {
		if pos >= len(data) {
			return errors.New("font container: truncated woff2 table directory")
		}
		flags := data[pos]
		pos++
		idx := int(flags & 0x3f)
		tv := int(flags >> 6)
		if idx == 63 {
			pos += 4
		}
		orig, n, ok := readBase128(data, pos)
		if !ok {
			return errors.New("font container: bad woff2 table directory")
		}
		pos += n
		if (idx == woff2TagGlyf || idx == woff2TagLoca) && tv == 0 || idx == woff2TagHmtx && tv == 1 {
			tl, n, ok := readBase128(data, pos)
			if !ok {
				return errors.New("font container: bad woff2 table directory")
			}
			pos += n
			declared += tl
		} else {
			declared += orig
		}
		if declared > maxDecodedSize {
			return errors.New("font container: decoded font too large")
		}
	}
	if pos > len(data) || compLen > int64(len(data)-pos) {
		return errors.New("font container: truncated woff2 data")
	}
	r := io.LimitReader(brotli.NewReader(bytes.NewReader(data[pos:pos+int(compLen)])), declared+1)
	got, err := io.Copy(io.Discard, r)
	if err != nil {
		return fmt.Errorf("font container: woff2 stream: %w", err)
	}
	if got > declared {
		return errors.New("font container: woff2 stream larger than declared")
	}
	return nil
}

// readBase128 reads a WOFF2 UIntBase128 value at pos, returning the value
// and the number of bytes consumed.
func readBase128(data []byte, pos int) (val int64, n int, ok bool) {
	for n < 5 {
		if pos+n >= len(data) {
			return 0, 0, false
		}
		b := data[pos+n]
		n++
		val = val<<7 | int64(b&0x7f)
		if b&0x80 == 0 {
			return val, n, true
		}
	}
	return 0, 0, false
}
