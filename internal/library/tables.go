package library

import (
	"cmp"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/amnuts/skrifttype/internal/store"
)

// This file reads the raw OpenType tables that golang.org/x/image/font/sfnt
// does not expose: OS/2 metrics, GSUB/GPOS feature lists and cmap
// coverage. Every reader is defensive — a malformed table yields zero
// values, never a panic, because fonts in the wild are frequently
// slightly broken.

// os2Metrics is the subset of the OS/2 table the catalogue stores.
type os2Metrics struct {
	WeightClass int
	WidthClass  int
	Italic      bool
}

// readOS2Metrics reads usWeightClass (offset 4), usWidthClass (offset 6)
// and fsSelection bit 0 (ITALIC, offset 62).
func readOS2Metrics(data []byte) (os2Metrics, bool) {
	tbl := findTable(data, "OS/2")
	if len(tbl) < 64 {
		return os2Metrics{}, false
	}
	return os2Metrics{
		WeightClass: int(binary.BigEndian.Uint16(tbl[4:6])),
		WidthClass:  int(binary.BigEndian.Uint16(tbl[6:8])),
		Italic:      binary.BigEndian.Uint16(tbl[62:64])&1 != 0,
	}, true
}

// readFeatureTags returns the distinct feature tags in a GSUB or GPOS
// table's FeatureList, in first-seen order.
//
// Layout: header { majorVersion u16, minorVersion u16, scriptListOffset
// u16, featureListOffset u16, ... }; FeatureList { featureCount u16,
// FeatureRecord[featureCount] { tag [4]byte, featureOffset u16 } }.
func readFeatureTags(data []byte, tableTag string) []string {
	tbl := findTable(data, tableTag)
	if len(tbl) < 10 {
		return nil
	}
	flOff := int(binary.BigEndian.Uint16(tbl[6:8]))
	if flOff == 0 || flOff+2 > len(tbl) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(tbl[flOff : flOff+2]))
	seen := make(map[string]struct{}, count)
	var out []string
	for i := 0; i < count; i++ {
		rec := flOff + 2 + i*6
		if rec+4 > len(tbl) {
			break
		}
		tag := strings.TrimRight(string(tbl[rec:rec+4]), " ")
		if !isPrintableTag(tag) {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func isPrintableTag(tag string) bool {
	if tag == "" {
		return false
	}
	for i := 0; i < len(tag); i++ {
		if tag[i] < 0x20 || tag[i] > 0x7e {
			return false
		}
	}
	return true
}

// readCmapRanges returns the inclusive codepoint ranges that map to a
// non-zero glyph. It prefers a full-repertoire format 12 subtable
// (platform 3/encoding 10 or platform 0/encoding 4|6) and falls back
// to a BMP format 4 subtable (3/1 or 0/3|0/1|0/2|0/0).
func readCmapRanges(data []byte) []store.CodepointRange {
	tbl := findTable(data, "cmap")
	if len(tbl) < 4 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(tbl[2:4]))
	var fmt12, fmt4 []byte
	for i := 0; i < n; i++ {
		rec := 4 + i*8
		if rec+8 > len(tbl) {
			break
		}
		platform := binary.BigEndian.Uint16(tbl[rec : rec+2])
		encoding := binary.BigEndian.Uint16(tbl[rec+2 : rec+4])
		off := int(binary.BigEndian.Uint32(tbl[rec+4 : rec+8]))
		if off < 0 || off+2 > len(tbl) {
			continue
		}
		sub := tbl[off:]
		format := binary.BigEndian.Uint16(sub[0:2])
		unicode := platform == 0 || (platform == 3 && (encoding == 1 || encoding == 10))
		if !unicode {
			continue
		}
		switch {
		case format == 12 && fmt12 == nil:
			fmt12 = sub
		case format == 4 && fmt4 == nil:
			fmt4 = sub
		}
	}
	var ranges []store.CodepointRange
	switch {
	case fmt12 != nil:
		ranges = cmapFormat12(fmt12)
	case fmt4 != nil:
		ranges = cmapFormat4(fmt4)
	}
	return mergeRanges(ranges)
}

// cmapFormat12: format u16, reserved u16, length u32, language u32,
// numGroups u32, then groups { startChar u32, endChar u32, startGlyph u32 }.
func cmapFormat12(sub []byte) []store.CodepointRange {
	if len(sub) < 16 {
		return nil
	}
	groups := int(binary.BigEndian.Uint32(sub[12:16]))
	var out []store.CodepointRange
	for i := 0; i < groups; i++ {
		g := 16 + i*12
		if g+12 > len(sub) {
			break
		}
		start := int(binary.BigEndian.Uint32(sub[g : g+4]))
		end := int(binary.BigEndian.Uint32(sub[g+4 : g+8]))
		glyph := binary.BigEndian.Uint32(sub[g+8 : g+12])
		if glyph == 0 {
			start++ // startChar maps to .notdef; the rest of the run does not
		}
		if start > end || end > 0x10FFFF {
			continue
		}
		out = append(out, store.CodepointRange{Lo: start, Hi: end})
	}
	return out
}

// cmapFormat4: format, length, language, segCountX2, searchRange,
// entrySelector, rangeShift (7 × u16), then endCode[segCount],
// reservedPad u16, startCode[segCount], idDelta[segCount],
// idRangeOffset[segCount], glyphIdArray[].
func cmapFormat4(sub []byte) []store.CodepointRange {
	if len(sub) < 14 {
		return nil
	}
	segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
	endBase := 14
	startBase := endBase + segCount*2 + 2
	deltaBase := startBase + segCount*2
	rangeBase := deltaBase + segCount*2
	if rangeBase+segCount*2 > len(sub) {
		return nil
	}
	u16 := func(off int) int {
		if off < 0 || off+2 > len(sub) {
			return 0
		}
		return int(binary.BigEndian.Uint16(sub[off : off+2]))
	}

	var out []store.CodepointRange
	runStart := -1
	flush := func(end int) {
		if runStart >= 0 {
			out = append(out, store.CodepointRange{Lo: runStart, Hi: end})
			runStart = -1
		}
	}
	for s := 0; s < segCount; s++ {
		end := u16(endBase + s*2)
		start := u16(startBase + s*2)
		delta := u16(deltaBase + s*2)
		roAddr := rangeBase + s*2
		ro := u16(roAddr)
		if start > end {
			continue
		}
		for c := start; c <= end; c++ {
			if c == 0xFFFF {
				break
			}
			var glyph int
			if ro == 0 {
				glyph = (c + delta) & 0xFFFF
			} else {
				g := u16(roAddr + ro + (c-start)*2)
				if g != 0 {
					g = (g + delta) & 0xFFFF
				}
				glyph = g
			}
			if glyph != 0 {
				if runStart < 0 {
					runStart = c
				}
				continue
			}
			flush(c - 1)
		}
		flush(min(end, 0xFFFE))
	}
	return out
}

// mergeRanges sorts and coalesces overlapping or adjacent ranges.
func mergeRanges(in []store.CodepointRange) []store.CodepointRange {
	if len(in) == 0 {
		return nil
	}
	slices.SortFunc(in, func(a, b store.CodepointRange) int { return cmp.Compare(a.Lo, b.Lo) })
	out := []store.CodepointRange{in[0]}
	for _, r := range in[1:] {
		last := &out[len(out)-1]
		if r.Lo <= last.Hi+1 {
			if r.Hi > last.Hi {
				last.Hi = r.Hi
			}
			continue
		}
		out = append(out, r)
	}
	return out
}
