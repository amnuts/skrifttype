package api

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/amnuts/skrifttype/internal/store"
)

// The frontend works in *families* (one row per family, with a styles
// list); the store works in *faces* (one row per font file/face). The
// catalogue groups faces into families and translates IDs both ways.
//
// A family's ID is the lowest face ID in it. That is stable across
// rescans (faces are upserted by path, and missing files are flagged
// rather than deleted), so collection membership and activation state
// keyed on faces map back to the same family ID every time.

// FontStyleDTO is one face within a family.
type FontStyleDTO struct {
	ID        int64  `json:"id"`
	StyleName string `json:"styleName"`
	Weight    int    `json:"weight"`
	Italic    bool   `json:"italic"`
	Path      string `json:"path"`
	// URL serves the font file to the webview (see FontFileMiddleware)
	// so previews work for fonts that are not installed.
	URL       string `json:"url"`
	Available bool   `json:"available"`
	// Activatable is false for system fonts and for WOFF/WOFF2 web fonts,
	// which can be previewed but never installed through GDI.
	Activatable bool `json:"activatable"`
}

// RangeDTO is an inclusive codepoint range.
type RangeDTO struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// FontDTO is the frontend-facing family record (FontRecord in TS).
type FontDTO struct {
	ID             int64          `json:"id"`
	Family         string         `json:"family"`
	CSSFamily      string         `json:"cssFamily"`
	Classification *string        `json:"classification"`
	Source         string         `json:"source"`
	Styles         []FontStyleDTO `json:"styles"`
	StyleCount     int            `json:"styleCount"`
	Designer       *string        `json:"designer"`
	Foundry        *string        `json:"foundry"`
	Version        *string        `json:"version"`
	Format         string         `json:"format"`
	Available      bool           `json:"available"`
	ParseError     *string        `json:"parseError"`
	IsVariable     bool           `json:"isVariable"`
	GsubFeatures   []string       `json:"gsubFeatures"`
	GposFeatures   []string       `json:"gposFeatures"`
	// InstalledByApp marks system fonts that skrifttype installed for all
	// users, i.e. the only ones it offers to uninstall.
	InstalledByApp bool `json:"installedByApp"`
	// UnicodeRanges is left empty in list results (it can be large for
	// CJK fonts); fetch it per family with LibraryService.GetGlyphRanges.
	UnicodeRanges []RangeDTO `json:"unicodeRanges"`
}

type family struct {
	dto    FontDTO
	faces  []int64
	ranges []store.CodepointRange
}

// catalogIndex is an immutable snapshot of the grouped catalogue.
type catalogIndex struct {
	families []*family // sorted by family name
	byID     map[int64]*family
	faceFam  map[int64]int64 // face ID -> family ID
}

// Catalog caches the grouped catalogue and rebuilds it lazily after
// Invalidate. Safe for concurrent use.
type Catalog struct {
	store *store.Store

	mu  sync.Mutex
	idx *catalogIndex
}

// NewCatalog constructs an empty (dirty) catalogue.
func NewCatalog(st *store.Store) *Catalog { return &Catalog{store: st} }

// Invalidate drops the cached index; the next read rebuilds it.
func (c *Catalog) Invalidate() {
	c.mu.Lock()
	c.idx = nil
	c.mu.Unlock()
}

func (c *Catalog) index(ctx context.Context) (*catalogIndex, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx != nil {
		return c.idx, nil
	}
	idx, err := buildIndex(ctx, c.store)
	if err != nil {
		return nil, err
	}
	c.idx = idx
	return idx, nil
}

// FacesOf returns the face IDs in the given families.
func (c *Catalog) FacesOf(ctx context.Context, familyIDs ...int64) ([]int64, error) {
	idx, err := c.index(ctx)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, id := range familyIDs {
		f, ok := idx.byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown font %d", id)
		}
		out = append(out, f.faces...)
	}
	return out, nil
}

// FamiliesOf maps face IDs to their (deduplicated, sorted) family IDs.
// Faces not in the catalogue (e.g. a disabled root) are dropped.
func (c *Catalog) FamiliesOf(ctx context.Context, faceIDs []int64) ([]int64, error) {
	idx, err := c.index(ctx)
	if err != nil {
		return nil, err
	}
	return idx.familiesOf(faceIDs), nil
}

func (idx *catalogIndex) familiesOf(faceIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(faceIDs))
	out := make([]int64, 0, len(faceIDs))
	for _, face := range faceIDs {
		fam, ok := idx.faceFam[face]
		if !ok {
			continue
		}
		if _, dup := seen[fam]; dup {
			continue
		}
		seen[fam] = struct{}{}
		out = append(out, fam)
	}
	slices.Sort(out)
	return out
}

func buildIndex(ctx context.Context, st *store.Store) (*catalogIndex, error) {
	fonts, err := st.ListFonts(ctx, store.FontFilter{})
	if err != nil {
		return nil, err
	}
	roots, err := st.ListWatchedRoots(ctx)
	if err != nil {
		return nil, err
	}
	enabledRoot := make(map[int64]bool, len(roots))
	for _, r := range roots {
		enabledRoot[r.ID] = r.Enabled
	}

	type key struct{ source, family string }
	groups := make(map[key][]store.Font)
	var order []key
	for _, f := range fonts {
		// Library fonts are shown only while their folder is enabled.
		if f.Source == store.FontSourceLibrary && !enabledRoot[f.WatchedRootID] {
			continue
		}
		k := key{source: f.Source, family: familyName(f)}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}

	installed, err := loadInstalled(ctx, st)
	if err != nil {
		return nil, err
	}

	idx := &catalogIndex{byID: make(map[int64]*family, len(groups)), faceFam: make(map[int64]int64, len(fonts))}
	for _, k := range order {
		fam := buildFamily(k.family, groups[k])
		if k.source != store.FontSourceLibrary {
			for _, st := range fam.dto.Styles {
				if installed[pathKey(st.Path)] {
					fam.dto.InstalledByApp = true
					break
				}
			}
		}
		idx.families = append(idx.families, fam)
		idx.byID[fam.dto.ID] = fam
		for _, face := range fam.faces {
			idx.faceFam[face] = fam.dto.ID
		}
	}
	slices.SortFunc(idx.families, func(a, b *family) int {
		if c := cmp.Compare(strings.ToLower(a.dto.Family), strings.ToLower(b.dto.Family)); c != 0 {
			return c
		}
		return cmp.Compare(a.dto.ID, b.dto.ID)
	})
	return idx, nil
}

func familyName(f store.Font) string {
	if n := strings.TrimSpace(f.Family); n != "" {
		return n
	}
	if n := strings.TrimSpace(f.FullName); n != "" {
		return n
	}
	return strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
}

func buildFamily(name string, faces []store.Font) *family {
	slices.SortFunc(faces, func(a, b store.Font) int {
		if a.Italic != b.Italic {
			if a.Italic {
				return 1
			}
			return -1
		}
		if c := cmp.Compare(weightOf(a), weightOf(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	fam := &family{}
	rep := representative(faces)
	dto := FontDTO{
		Family:       name,
		Source:       rep.Source,
		Format:       rep.Format,
		Designer:     optString(rep.Designer),
		Foundry:      optString(rep.Foundry),
		Version:      optString(rep.VersionString),
		GsubFeatures: []string{},
		GposFeatures: []string{},
		Styles:       make([]FontStyleDTO, 0, len(faces)),
		// Always an array (never null) so the TS side can iterate it.
		UnicodeRanges: []RangeDTO{},
	}
	classCount := map[string]int{}
	allErrored := true
	var firstErr string
	var ranges []store.CodepointRange
	for _, f := range faces {
		if dto.ID == 0 || f.ID < dto.ID {
			dto.ID = f.ID
		}
		fam.faces = append(fam.faces, f.ID)
		style := f.Subfamily
		if style == "" {
			style = "Regular"
		}
		dto.Styles = append(dto.Styles, FontStyleDTO{
			ID: f.ID, StyleName: style, Weight: weightOf(f), Italic: f.Italic,
			Path: f.Path, URL: fontFileURL(f.ID), Available: f.Available,
			Activatable: f.Source == store.FontSourceLibrary && f.Format != "woff" && f.Format != "woff2",
		})
		dto.Available = dto.Available || f.Available
		dto.IsVariable = dto.IsVariable || f.IsVariable
		dto.GsubFeatures = appendUnique(dto.GsubFeatures, f.GsubFeatures...)
		dto.GposFeatures = appendUnique(dto.GposFeatures, f.GposFeatures...)
		if f.Classification != "" {
			classCount[f.Classification]++
		}
		if f.ParseError == "" {
			allErrored = false
		} else if firstErr == "" {
			firstErr = f.ParseError
		}
		ranges = append(ranges, f.UnicodeCodepoints...)
	}
	dto.StyleCount = len(dto.Styles)
	dto.CSSFamily = fmt.Sprintf("skrift-%d", dto.ID)
	if allErrored && firstErr != "" {
		dto.ParseError = &firstErr
	}
	if c := majority(classCount, rep.Classification); c != "" {
		dto.Classification = &c
	}
	fam.dto = dto
	fam.ranges = mergeRanges(ranges)
	return fam
}

// representative picks the face whose metadata describes the family:
// the upright face closest to regular weight.
func representative(faces []store.Font) store.Font {
	best := faces[0]
	score := func(f store.Font) int {
		s := abs(weightOf(f) - 400)
		if f.Italic {
			s += 1000
		}
		if !f.Available {
			s += 2000
		}
		return s
	}
	for _, f := range faces[1:] {
		if score(f) < score(best) {
			best = f
		}
	}
	return best
}

// majority returns the most common classification; ties go to prefer
// (the representative face's), then alphabetically for determinism.
func majority(counts map[string]int, prefer string) string {
	best, bestN := "", 0
	for c, n := range counts {
		switch {
		case n > bestN,
			n == bestN && c == prefer,
			n == bestN && best != prefer && c < best:
			best, bestN = c, n
		}
	}
	return best
}

func weightOf(f store.Font) int {
	if f.WeightClass > 0 {
		return f.WeightClass
	}
	return 400
}

func fontFileURL(faceID int64) string { return fmt.Sprintf("%s%d", fontFilePrefix, faceID) }

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func appendUnique(dst []string, src ...string) []string {
	for _, s := range src {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}

func mergeRanges(in []store.CodepointRange) []store.CodepointRange {
	if len(in) == 0 {
		return nil
	}
	rs := slices.Clone(in)
	slices.SortFunc(rs, func(a, b store.CodepointRange) int { return cmp.Compare(a.Lo, b.Lo) })
	out := []store.CodepointRange{rs[0]}
	for _, r := range rs[1:] {
		last := &out[len(out)-1]
		if r.Lo <= last.Hi+1 {
			last.Hi = max(last.Hi, r.Hi)
			continue
		}
		out = append(out, r)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
