package library

import (
	"encoding/binary"
	"testing"
)

// sfntWithOS2 builds a minimal sfnt byte slice containing only an OS/2
// table with the given sFamilyClass and PANOSE bytes.
func sfntWithOS2(familyClass uint16, panose [10]byte) []byte {
	os2 := make([]byte, 78)
	binary.BigEndian.PutUint16(os2[30:32], familyClass)
	copy(os2[32:42], panose[:])
	const dir = 12 + 16
	data := make([]byte, dir+len(os2))
	binary.BigEndian.PutUint16(data[4:6], 1) // numTables
	copy(data[12:16], "OS/2")
	binary.BigEndian.PutUint32(data[20:24], dir)
	binary.BigEndian.PutUint32(data[24:28], uint32(len(os2)))
	copy(data[dir:], os2)
	return data
}

func TestClassifyFont_Fallbacks(t *testing.T) {
	cases := []struct {
		name        string
		familyClass uint16
		panose      [10]byte
		family      string
		want        string
	}{
		{"IBM class wins", 8 << 8, [10]byte{2, 4}, "X", "sans"},
		{"PANOSE cove serif", 0, [10]byte{2, 2}, "Caslon", "serif"},
		{"PANOSE normal sans", 0, [10]byte{2, 11}, "Helvetica", "sans"},
		{"PANOSE rounded sans", 0, [10]byte{2, 15}, "Rounded", "sans"},
		{"PANOSE script", 0, [10]byte{3}, "Brush", "script"},
		{"name: sans serif is sans", 0, [10]byte{}, "Noto Sans Serif Display", "sans"},
		{"name: slab", 0, [10]byte{}, "Roboto Slab", "serif"},
		{"name: mono", 0, [10]byte{}, "Fira Code", "mono"},
		{"unknown", 0, [10]byte{}, "Zapfino-ish", ""},
	}
	for _, c := range cases {
		if got := classifyFont(sfntWithOS2(c.familyClass, c.panose), c.family); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
