package library

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font/sfnt"
)

// NewSfntParser returns the production Parser implementation. It uses
// golang.org/x/image/font/sfnt to read the OpenType name table, and
// raw table readers (tables.go) for OS/2, fvar, GSUB, GPOS and cmap.
func NewSfntParser() Parser { return &sfntParser{} }

type sfntParser struct{}

// Parse returns one FontRecord per face in the file. Plain sfnt files and
// WOFF/WOFF2 wrappers yield one record, TTC/OTC collections one per member.
// Each face is normalised to standalone sfnt bytes by SplitFaces so the
// per-face extraction below is identical for every container.
func (p *sfntParser) Parse(path string) ([]FontRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font file: %w", err)
	}
	faces, container, err := SplitFaces(data)
	if err != nil {
		return nil, fmt.Errorf("split faces: %w", err)
	}
	records := make([]FontRecord, 0, len(faces))
	for i, face := range faces {
		rec, err := parseFace(face, faceFormat(container, face), i)
		if err != nil {
			return nil, fmt.Errorf("face %d: %w", i, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// faceFormat names the stored format of one face: the container name for
// web fonts, otherwise ttf/otf (or ttc/otc inside a collection) by outline
// type.
func faceFormat(c Container, face []byte) string {
	switch c {
	case ContainerWOFF:
		return "woff"
	case ContainerWOFF2:
		return "woff2"
	case ContainerTTC:
		if OutlineExt(face) == ".otf" {
			return "otc"
		}
		return "ttc"
	}
	if OutlineExt(face) == ".otf" {
		return "otf"
	}
	return "ttf"
}

// parseFace runs the full metadata extraction on one standalone sfnt face.
func parseFace(data []byte, format string, faceIndex int) (FontRecord, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return FontRecord{}, fmt.Errorf("sfnt parse: %w", err)
	}
	rec := extractRecord(f, format, faceIndex)
	rec.Classification = classifyFont(data, rec.Family)
	if m, ok := readOS2Metrics(data); ok {
		rec.WeightClass, rec.WidthClass, rec.Italic = m.WeightClass, m.WidthClass, m.Italic
	}
	if !rec.Italic {
		sub := strings.ToLower(rec.Subfamily)
		rec.Italic = strings.Contains(sub, "italic") || strings.Contains(sub, "oblique")
	}
	rec.IsVariable = findTable(data, "fvar") != nil
	rec.GsubFeatures = readFeatureTags(data, "GSUB")
	rec.GposFeatures = readFeatureTags(data, "GPOS")
	rec.Codepoints = readCmapRanges(data)
	return rec, nil
}

// extractRecord pulls every name-table field we care about into a
// FontRecord. Errors from individual Name() calls are swallowed
// (they're typically "name table missing this ID"), leaving the
// field as the empty string.
func extractRecord(f *sfnt.Font, format string, faceIndex int) FontRecord {
	var buf sfnt.Buffer
	name := func(id sfnt.NameID) string {
		s, _ := f.Name(&buf, id)
		return s
	}
	// Prefer the typographic (nameID 16/17) names: the legacy family
	// (nameID 1) splits large families into four-style groups such as
	// "Inter Medium" / "Regular" or "Caslon Pro Bold" / "Italic".
	pick := func(preferred, legacy sfnt.NameID) string {
		if v := strings.TrimSpace(name(preferred)); v != "" {
			return v
		}
		return name(legacy)
	}
	return FontRecord{
		Family:         pick(sfnt.NameIDTypographicFamily, sfnt.NameIDFamily),
		Subfamily:      pick(sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily),
		FullName:       name(sfnt.NameIDFull),
		PostscriptName: name(sfnt.NameIDPostScript),
		Designer:       name(sfnt.NameIDDesigner),
		Foundry:        name(sfnt.NameIDManufacturer),
		LicenseText:    name(sfnt.NameIDLicense),
		VersionString:  name(sfnt.NameIDVersion),
		Format:         format,
		FaceIndex:      faceIndex,
	}
}

// formatFromExt maps a file extension to the format code stored in
// store.Font.Format, case-insensitively. It is the scanner's fallback when
// a file cannot be parsed; Parse itself derives the format from the
// container and outline type.
func formatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ttf":
		return "ttf"
	case ".otf":
		return "otf"
	case ".ttc":
		return "ttc"
	case ".otc":
		return "otc"
	case ".woff":
		return "woff"
	case ".woff2":
		return "woff2"
	default:
		return "ttf"
	}
}

// classifyFont derives a classification string from the raw font bytes
// and the parsed family name. Returns "serif", "sans", "mono",
// "script", "display", or "" when classification is indeterminate.
//
// Priority order:
//  1. post.isFixedPitch → "mono" (authoritative for fixed-pitch)
//  2. Family name heuristic → "mono" (Courier, Code, Terminal, etc.)
//  3. PANOSE bFamilyType → "script" / "display"
//  4. OS/2 sFamilyClass high byte → broad category
//  5. PANOSE bSerifStyle for Latin text faces → "serif" / "sans"
//  6. Family-name hints
func classifyFont(data []byte, familyName string) string {
	if postIsFixedPitch(data) {
		return "mono"
	}
	lower := strings.ToLower(familyName)
	for _, hint := range []string{"mono", "code", "console", "terminal", "fixed", "courier"} {
		if strings.Contains(lower, hint) {
			return "mono"
		}
	}

	familyClass, panose, ok := readOS2(data)
	if !ok {
		return ""
	}

	// PANOSE bFamilyType: 3 = hand-written, 4/5 = decorative/symbolic
	switch panose[0] {
	case 3:
		return "script"
	case 4, 5:
		return "display"
	}

	// sFamilyClass high byte is the IBM font class
	switch familyClass >> 8 {
	case 1, 2, 3, 4, 5, 7:
		return "serif"
	case 8:
		return "sans"
	case 9, 12, 13:
		return "display"
	case 10:
		return "script"
	}

	// Many fonts leave sFamilyClass at 0 ("no classification"). For
	// Latin text faces (bFamilyType 2) PANOSE bSerifStyle still says
	// whether there are serifs: 2-10 are serif forms, 11-15 are sans
	// (normal, obtuse, perpendicular, flared, rounded).
	if panose[0] == 2 {
		switch {
		case panose[1] >= 2 && panose[1] <= 10:
			return "serif"
		case panose[1] >= 11 && panose[1] <= 15:
			return "sans"
		}
	}

	// Last resort: the family name. Check "sans" before "serif" so
	// "Sans Serif" families land in sans.
	switch {
	case strings.Contains(lower, "sans"), strings.Contains(lower, "grotesk"), strings.Contains(lower, "gothic"):
		return "sans"
	case strings.Contains(lower, "serif"), strings.Contains(lower, "slab"), strings.Contains(lower, "roman"):
		return "serif"
	case strings.Contains(lower, "script"), strings.Contains(lower, "hand"):
		return "script"
	}
	return ""
}

// readOS2 extracts sFamilyClass and PANOSE from the OS/2 table.
// The OS/2 table layout (version 0+):
//
//	offset 30: sFamilyClass (int16)
//	offset 32: PANOSE[10]
func readOS2(data []byte) (familyClass int16, panose [10]byte, ok bool) {
	tbl := findTable(data, "OS/2")
	if len(tbl) < 42 {
		return 0, panose, false
	}
	familyClass = int16(binary.BigEndian.Uint16(tbl[30:32]))
	copy(panose[:], tbl[32:42])
	return familyClass, panose, true
}

// postIsFixedPitch reads the isFixedPitch field from the post table.
// post table layout: version(4) italicAngle(4) underlinePos(2)
// underlineThick(2) isFixedPitch(4) — non-zero means fixed pitch.
func postIsFixedPitch(data []byte) bool {
	tbl := findTable(data, "post")
	if len(tbl) < 16 {
		return false
	}
	return binary.BigEndian.Uint32(tbl[12:16]) != 0
}

// findTable locates a named table within raw OpenType/TrueType bytes
// and returns its data slice. Returns nil if the tag is not found or
// the table data is out of bounds.
func findTable(data []byte, tag string) []byte {
	if len(data) < 12 {
		return nil
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	dirEnd := 12 + numTables*16
	if len(data) < dirEnd {
		return nil
	}
	tagB := []byte(tag)
	for i := 0; i < numTables; i++ {
		entry := data[12+i*16:]
		if bytes.Equal(entry[:4], tagB) {
			offset := int(binary.BigEndian.Uint32(entry[8:12]))
			length := int(binary.BigEndian.Uint32(entry[12:16]))
			end := offset + length
			if offset < 0 || end > len(data) {
				return nil
			}
			return data[offset:end]
		}
	}
	return nil
}
