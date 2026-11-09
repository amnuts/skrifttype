package store_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

func sampleLibraryFont(path string) store.Font {
	return store.Font{
		Path:           path,
		FileSize:       100_000,
		FileMtime:      1_700_000_000,
		Family:         "Inter",
		Subfamily:      "Regular",
		FullName:       "Inter Regular",
		PostscriptName: "Inter-Regular",
		Format:         "ttf",
		FaceIndex:      0,
		WeightClass:    400,
		WidthClass:     5,
		Italic:         false,
		IsVariable:     false,
		Classification: "sans",
		Designer:       "Rasmus Andersson",
		Foundry:        "Rasmus Andersson",
		LicenseText:    "OFL",
		VersionString:  "Version 4.0",
		GsubFeatures:   []string{"liga", "dlig", "ss01"},
		GposFeatures:   []string{"kern"},
		VariationAxes:  nil,
		UnicodeRanges:  []string{"Latin", "Cyrillic"},
		UnicodeCodepoints: []store.CodepointRange{
			{Lo: 0x0020, Hi: 0x007E},
			{Lo: 0x00A0, Hi: 0x017F},
		},
		Source:        store.FontSourceLibrary,
		WatchedRootID: 0,
		Available:     true,
	}
}

func TestUpsertFont_InsertNew(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	in := sampleLibraryFont(`D:\fonts\Inter-Regular.ttf`)
	id, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := s.GetFontByID(ctx, id)
	if err != nil {
		t.Fatalf("GetFontByID: %v", err)
	}
	if got.Path != in.Path {
		t.Errorf("Path: got %q, want %q", got.Path, in.Path)
	}
	if got.Family != in.Family {
		t.Errorf("Family: got %q, want %q", got.Family, in.Family)
	}
	if got.WeightClass != in.WeightClass {
		t.Errorf("WeightClass: got %d, want %d", got.WeightClass, in.WeightClass)
	}
	if !reflect.DeepEqual(got.GsubFeatures, in.GsubFeatures) {
		t.Errorf("GsubFeatures: got %v, want %v", got.GsubFeatures, in.GsubFeatures)
	}
	if !reflect.DeepEqual(got.UnicodeCodepoints, in.UnicodeCodepoints) {
		t.Errorf("UnicodeCodepoints: got %v, want %v", got.UnicodeCodepoints, in.UnicodeCodepoints)
	}
	if got.FirstSeenAt == 0 || got.LastSeenAt == 0 {
		t.Errorf("expected non-zero FirstSeenAt/LastSeenAt, got %d/%d", got.FirstSeenAt, got.LastSeenAt)
	}
}

func TestUpsertFont_UpdateExistingByPath(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	in := sampleLibraryFont(`D:\fonts\Inter-Regular.ttf`)
	in.FileSize = 100_000
	id1, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("first UpsertFont: %v", err)
	}

	in.FileSize = 110_000
	in.Family = "Inter v2"
	id2, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("second UpsertFont: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected same ID on upsert by path; got %d then %d", id1, id2)
	}

	got, err := s.GetFontByID(ctx, id2)
	if err != nil {
		t.Fatalf("GetFontByID: %v", err)
	}
	if got.FileSize != 110_000 {
		t.Errorf("FileSize: expected updated 110000, got %d", got.FileSize)
	}
	if got.Family != "Inter v2" {
		t.Errorf("Family: expected updated, got %q", got.Family)
	}
}

func TestGetFontByPathFace(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	want := sampleLibraryFont(`D:\fonts\Roboto-Regular.ttf`)
	if _, err := s.UpsertFont(ctx, want); err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}

	got, err := s.GetFontByPathFace(ctx, want.Path, 0)
	if err != nil {
		t.Fatalf("GetFontByPathFace: %v", err)
	}
	if got.Path != want.Path {
		t.Errorf("Path: got %q, want %q", got.Path, want.Path)
	}

	_, err = s.GetFontByPathFace(ctx, `D:\does\not\exist.ttf`, 0)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestListFonts_FilterBySourceAndAvailable(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	libFont := sampleLibraryFont(`D:\fonts\one.ttf`)
	if _, err := s.UpsertFont(ctx, libFont); err != nil {
		t.Fatalf("upsert lib: %v", err)
	}

	sysFont := sampleLibraryFont(`C:\Windows\Fonts\arial.ttf`)
	sysFont.Source = store.FontSourceSystemHKLM
	if _, err := s.UpsertFont(ctx, sysFont); err != nil {
		t.Fatalf("upsert sys: %v", err)
	}

	gone := sampleLibraryFont(`D:\fonts\missing.ttf`)
	gone.Available = false
	if _, err := s.UpsertFont(ctx, gone); err != nil {
		t.Fatalf("upsert gone: %v", err)
	}

	all, err := s.ListFonts(ctx, store.FontFilter{})
	if err != nil {
		t.Fatalf("ListFonts (no filter): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 fonts, got %d", len(all))
	}

	libOnly, err := s.ListFonts(ctx, store.FontFilter{Source: store.FontSourceLibrary})
	if err != nil {
		t.Fatalf("ListFonts (library): %v", err)
	}
	if len(libOnly) != 2 {
		t.Errorf("expected 2 library fonts, got %d", len(libOnly))
	}

	available, err := s.ListFonts(ctx, store.FontFilter{AvailableOnly: true})
	if err != nil {
		t.Fatalf("ListFonts (availableOnly): %v", err)
	}
	if len(available) != 2 {
		t.Errorf("expected 2 available fonts, got %d", len(available))
	}
}

func TestListFonts_Pagination(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	for i := 0; i < 5; i++ {
		f := sampleLibraryFont("")
		// Use distinct paths so each upsert is a new row.
		f.Path = `D:\fonts\page-` + string(rune('a'+i)) + `.ttf`
		f.Family = `Page-` + string(rune('a'+i))
		if _, err := s.UpsertFont(ctx, f); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
	}

	page, err := s.ListFonts(ctx, store.FontFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListFonts (page): %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 rows on page, got %d", len(page))
	}
}

func TestDeleteFont(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	in := sampleLibraryFont(`D:\fonts\to-delete.ttf`)
	id, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	if err := s.DeleteFont(ctx, id); err != nil {
		t.Fatalf("DeleteFont: %v", err)
	}
	if _, err := s.GetFontByID(ctx, id); err == nil {
		t.Fatal("expected error fetching deleted font")
	}
}

func TestMarkFontUnavailable(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	in := sampleLibraryFont(`D:\fonts\to-mark.ttf`)
	id, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	if err := s.MarkFontUnavailable(ctx, id); err != nil {
		t.Fatalf("MarkFontUnavailable: %v", err)
	}
	got, err := s.GetFontByID(ctx, id)
	if err != nil {
		t.Fatalf("GetFontByID: %v", err)
	}
	if got.Available {
		t.Error("expected Available=false after MarkFontUnavailable")
	}
}

func TestUpsertFont_NullableFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// All optional/nullable fields zero-valued. The store must map
	// them to SQL NULL on the way in and back to zero values on the
	// way out without leaking sql.NullString / sql.NullInt64 through
	// the public API.
	in := store.Font{
		Path:      `D:\fonts\sparse.ttf`,
		FileSize:  1234,
		FileMtime: 1_700_000_000,
		Family:    "Sparse",
		Subfamily: "Regular",
		FullName:  "Sparse Regular",
		Format:    "ttf",
		Source:    store.FontSourceLibrary,
		Available: true,
		// Everything else is the Go zero value: empty strings, 0 ints,
		// nil slices, false bools.
	}
	id, err := s.UpsertFont(ctx, in)
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	got, err := s.GetFontByID(ctx, id)
	if err != nil {
		t.Fatalf("GetFontByID: %v", err)
	}

	if got.SHA256 != "" {
		t.Errorf("SHA256: got %q, want empty", got.SHA256)
	}
	if got.PostscriptName != "" {
		t.Errorf("PostscriptName: got %q, want empty", got.PostscriptName)
	}
	if got.WeightClass != 0 {
		t.Errorf("WeightClass: got %d, want 0", got.WeightClass)
	}
	if got.WidthClass != 0 {
		t.Errorf("WidthClass: got %d, want 0", got.WidthClass)
	}
	if got.Classification != "" {
		t.Errorf("Classification: got %q, want empty", got.Classification)
	}
	if got.Designer != "" {
		t.Errorf("Designer: got %q, want empty", got.Designer)
	}
	if got.Foundry != "" {
		t.Errorf("Foundry: got %q, want empty", got.Foundry)
	}
	if got.LicenseText != "" {
		t.Errorf("LicenseText: got %q, want empty", got.LicenseText)
	}
	if got.VersionString != "" {
		t.Errorf("VersionString: got %q, want empty", got.VersionString)
	}
	if got.ParseError != "" {
		t.Errorf("ParseError: got %q, want empty", got.ParseError)
	}
	if got.WatchedRootID != 0 {
		t.Errorf("WatchedRootID: got %d, want 0", got.WatchedRootID)
	}
	// JSON-backed slices: per the Font docblock, nil round-trips to a
	// non-nil zero-length slice. We assert length 0, not nil.
	if len(got.GsubFeatures) != 0 {
		t.Errorf("GsubFeatures: expected length 0, got %v", got.GsubFeatures)
	}
	if len(got.GposFeatures) != 0 {
		t.Errorf("GposFeatures: expected length 0, got %v", got.GposFeatures)
	}
	if len(got.VariationAxes) != 0 {
		t.Errorf("VariationAxes: expected length 0, got %v", got.VariationAxes)
	}
	if len(got.UnicodeRanges) != 0 {
		t.Errorf("UnicodeRanges: expected length 0, got %v", got.UnicodeRanges)
	}
	if len(got.UnicodeCodepoints) != 0 {
		t.Errorf("UnicodeCodepoints: expected length 0, got %v", got.UnicodeCodepoints)
	}
}

func TestSearchFonts_FindsByFamily(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a := sampleLibraryFont(`D:\fonts\inter.ttf`)
	a.Family = "Inter"
	a.FullName = "Inter Regular"
	if _, err := s.UpsertFont(ctx, a); err != nil {
		t.Fatalf("upsert a: %v", err)
	}

	b := sampleLibraryFont(`D:\fonts\roboto.ttf`)
	b.Family = "Roboto"
	b.FullName = "Roboto Regular"
	b.PostscriptName = "Roboto-Regular"
	if _, err := s.UpsertFont(ctx, b); err != nil {
		t.Fatalf("upsert b: %v", err)
	}

	hits, err := s.SearchFonts(ctx, "Inter", 10, 0)
	if err != nil {
		t.Fatalf("SearchFonts: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for 'Inter', got %d", len(hits))
	}
	if hits[0].Family != "Inter" {
		t.Errorf("hit family: got %q, want Inter", hits[0].Family)
	}
}

func TestSearchFonts_TolerantOfSpecialChars(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a := sampleLibraryFont(`D:\fonts\source-serif.otf`)
	a.Family = "Source Serif"
	a.FullName = "Source Serif Pro"
	if _, err := s.UpsertFont(ctx, a); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// User pastes a query with a literal double-quote and parens —
	// sanitiser must strip them, FTS5 must not panic.
	hits, err := s.SearchFonts(ctx, `"Source" (Serif)`, 10, 0)
	if err != nil {
		t.Fatalf("SearchFonts with special chars: %v", err)
	}
	if len(hits) == 0 {
		t.Errorf("expected at least one hit for 'Source Serif'")
	}
}

func TestSearchFonts_PreservesUnicodeInTokens(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a := sampleLibraryFont(`D:\fonts\muller.otf`)
	a.Family = "Müller Display"
	a.FullName = "Müller Display Regular"
	a.PostscriptName = "MullerDisplay-Regular"
	a.Designer = "Müller"
	if _, err := s.UpsertFont(ctx, a); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// FTS5's default unicode61 tokenizer folds diacritics on the
	// index side, so a search for "Muller" should match the row whose
	// designer/family contains "Müller".
	hits, err := s.SearchFonts(ctx, "Muller", 10, 0)
	if err != nil {
		t.Fatalf("SearchFonts ASCII fold: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected ASCII 'Muller' to match unicode 'Müller' via tokenizer fold")
	}

	// And searching with the original umlaut must also work — this
	// is the regression check: the sanitiser must not strip the ü.
	hits, err = s.SearchFonts(ctx, "Müller", 10, 0)
	if err != nil {
		t.Fatalf("SearchFonts unicode: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected unicode 'Müller' to match itself; sanitiser may be stripping non-ASCII")
	}
}

func TestUpsertFontTwoFacesSamePath(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	f0 := sampleLibraryFont(`D:\fonts\Collection.ttc`)
	f0.FaceIndex = 0
	f0.Subfamily = "Regular"
	f1 := f0
	f1.FaceIndex = 1
	f1.Subfamily = "Bold"

	id0, err := s.UpsertFont(ctx, f0)
	if err != nil {
		t.Fatalf("upsert face 0: %v", err)
	}
	id1, err := s.UpsertFont(ctx, f1)
	if err != nil {
		t.Fatalf("upsert face 1: %v", err)
	}
	if id0 == id1 {
		t.Fatalf("faces share id %d", id0)
	}

	f1.FileSize = 123
	again, err := s.UpsertFont(ctx, f1)
	if err != nil {
		t.Fatalf("re-upsert face 1: %v", err)
	}
	if again != id1 {
		t.Errorf("face 1 id changed: %d -> %d", id1, again)
	}
	got, err := s.GetFontByPathFace(ctx, f1.Path, 1)
	if err != nil {
		t.Fatalf("GetFontByPathFace: %v", err)
	}
	if got.ID != id1 || got.Subfamily != "Bold" || got.FileSize != 123 {
		t.Errorf("unexpected face 1 row: %+v", got)
	}
	if _, err := s.GetFontByPathFace(ctx, f1.Path, 2); err == nil {
		t.Error("expected error for missing face")
	}
}

func TestListFontsByPath(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	p := `D:\fonts\Collection.ttc`
	for _, idx := range []int{2, 0, 1} {
		f := sampleLibraryFont(p)
		f.FaceIndex = idx
		if _, err := s.UpsertFont(ctx, f); err != nil {
			t.Fatalf("upsert %d: %v", idx, err)
		}
	}
	if _, err := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\Other.ttf`)); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListFontsByPath(ctx, p)
	if err != nil {
		t.Fatalf("ListFontsByPath: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d faces, want 3", len(got))
	}
	for i, f := range got {
		if f.FaceIndex != i {
			t.Errorf("position %d has face_index %d (want ascending order)", i, f.FaceIndex)
		}
	}
	none, err := s.ListFontsByPath(ctx, `D:\nope.ttc`)
	if err != nil || len(none) != 0 {
		t.Errorf("missing path: %v, %v", none, err)
	}
}
