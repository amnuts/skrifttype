package store_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

func TestActivations_AddManualBringsActive(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	fid, err := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\m.ttf`))
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}

	became, err := s.AddActivationSource(ctx, fid, store.ActivationSource{Kind: store.ActivationKindManual})
	if err != nil {
		t.Fatalf("AddActivationSource: %v", err)
	}
	if !became {
		t.Error("expected becameActive=true on first source")
	}

	// Re-adding the same manual source is a no-op (PK conflict ignored).
	became, err = s.AddActivationSource(ctx, fid, store.ActivationSource{Kind: store.ActivationKindManual})
	if err != nil {
		t.Fatalf("re-AddActivationSource: %v", err)
	}
	if became {
		t.Error("expected becameActive=false on duplicate source")
	}

	active, err := s.IsActive(ctx, fid)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("expected font to be active")
	}
}

func TestActivations_RemoveLastSourceBringsInactive(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	fid, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\m.ttf`))
	manualSrc := store.ActivationSource{Kind: store.ActivationKindManual}

	if _, err := s.AddActivationSource(ctx, fid, manualSrc); err != nil {
		t.Fatalf("AddActivationSource: %v", err)
	}

	became, err := s.RemoveActivationSource(ctx, fid, manualSrc)
	if err != nil {
		t.Fatalf("RemoveActivationSource: %v", err)
	}
	if !became {
		t.Error("expected becameInactive=true on last source removal")
	}

	active, err := s.IsActive(ctx, fid)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("expected font to be inactive")
	}
}

func TestActivations_RefcountAcrossSources(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	fid, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\refcount.ttf`))
	c, _ := s.CreateCollection(ctx, "X")

	manualSrc := store.ActivationSource{Kind: store.ActivationKindManual}
	colSrc := store.ActivationSource{Kind: store.ActivationKindCollection, SourceID: sql.NullInt64{Int64: c.ID, Valid: true}}

	// Both sources active.
	if _, err := s.AddActivationSource(ctx, fid, manualSrc); err != nil {
		t.Fatalf("Add manual: %v", err)
	}
	if _, err := s.AddActivationSource(ctx, fid, colSrc); err != nil {
		t.Fatalf("Add collection: %v", err)
	}

	// Removing one source must NOT deactivate the font.
	became, err := s.RemoveActivationSource(ctx, fid, manualSrc)
	if err != nil {
		t.Fatalf("RemoveActivationSource: %v", err)
	}
	if became {
		t.Error("becameInactive should be false while another source is still active")
	}
	active, err := s.IsActive(ctx, fid)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("font should still be active via the collection source")
	}

	// Listing sources confirms only the collection source remains.
	srcs, err := s.ListActivationSourcesForFont(ctx, fid)
	if err != nil {
		t.Fatalf("ListActivationSourcesForFont: %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("expected 1 remaining source, got %d", len(srcs))
	}
	if srcs[0].Kind != store.ActivationKindCollection {
		t.Errorf("remaining source kind: got %q, want %q", srcs[0].Kind, store.ActivationKindCollection)
	}
}

func TestActivations_ReplaceActivationSet_ComputesDiff(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	c, _ := s.CreateCollection(ctx, "Set")
	colSrc := store.ActivationSource{Kind: store.ActivationKindCollection, SourceID: sql.NullInt64{Int64: c.ID, Valid: true}}

	// Three fonts in play.
	a, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\a.ttf`))
	b, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\b.ttf`))
	cFont, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\c.ttf`))

	// Initial set: {a, b}.
	diff, err := s.ReplaceActivationSet(ctx, colSrc, []int64{a, b})
	if err != nil {
		t.Fatalf("ReplaceActivationSet (initial): %v", err)
	}
	sort.Slice(diff.NewlyActive, func(i, j int) bool { return diff.NewlyActive[i] < diff.NewlyActive[j] })
	if got, want := diff.NewlyActive, []int64{a, b}; !equalInt64s(got, want) {
		t.Errorf("NewlyActive (initial): got %v, want %v", got, want)
	}
	if len(diff.NewlyInactive) != 0 {
		t.Errorf("NewlyInactive (initial): got %v, want []", diff.NewlyInactive)
	}

	// Replace with {b, c}: a goes inactive, c goes active, b stays.
	diff, err = s.ReplaceActivationSet(ctx, colSrc, []int64{b, cFont})
	if err != nil {
		t.Fatalf("ReplaceActivationSet (swap): %v", err)
	}
	if got, want := diff.NewlyInactive, []int64{a}; !equalInt64s(got, want) {
		t.Errorf("NewlyInactive (swap): got %v, want %v", got, want)
	}
	if got, want := diff.NewlyActive, []int64{cFont}; !equalInt64s(got, want) {
		t.Errorf("NewlyActive (swap): got %v, want %v", got, want)
	}
}

func TestActivations_ReplaceActivationSet_PreservesOtherSources(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	c, _ := s.CreateCollection(ctx, "Set")
	colSrc := store.ActivationSource{Kind: store.ActivationKindCollection, SourceID: sql.NullInt64{Int64: c.ID, Valid: true}}
	manualSrc := store.ActivationSource{Kind: store.ActivationKindManual}

	a, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\a.ttf`))

	// Manually activate a.
	if _, err := s.AddActivationSource(ctx, a, manualSrc); err != nil {
		t.Fatalf("Add manual: %v", err)
	}
	// Then activate via collection.
	diff, err := s.ReplaceActivationSet(ctx, colSrc, []int64{a})
	if err != nil {
		t.Fatalf("ReplaceActivationSet: %v", err)
	}
	// a was already active via manual, so net change is zero.
	if len(diff.NewlyActive) != 0 || len(diff.NewlyInactive) != 0 {
		t.Errorf("expected zero net diff, got NewlyActive=%v NewlyInactive=%v", diff.NewlyActive, diff.NewlyInactive)
	}

	// Now empty the collection set. a is still manually active.
	diff, err = s.ReplaceActivationSet(ctx, colSrc, nil)
	if err != nil {
		t.Fatalf("ReplaceActivationSet (empty): %v", err)
	}
	if len(diff.NewlyInactive) != 0 {
		t.Errorf("a should still be active via manual; got NewlyInactive=%v", diff.NewlyInactive)
	}
	active, err := s.IsActive(ctx, a)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("a should still be active via the manual source")
	}
}

func TestActivations_ListActiveFontIDs(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\a.ttf`))
	b, _ := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\b.ttf`))
	_, _ = s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\c.ttf`))

	manualSrc := store.ActivationSource{Kind: store.ActivationKindManual}
	if _, err := s.AddActivationSource(ctx, a, manualSrc); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.AddActivationSource(ctx, b, manualSrc); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	ids, err := s.ListActiveFontIDs(ctx)
	if err != nil {
		t.Fatalf("ListActiveFontIDs: %v", err)
	}
	if got, want := ids, []int64{a, b}; !equalInt64s(got, want) {
		t.Errorf("ListActiveFontIDs: got %v, want %v", got, want)
	}
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]int64(nil), a...)
	bc := append([]int64(nil), b...)
	sort.Slice(ac, func(i, j int) bool { return ac[i] < ac[j] })
	sort.Slice(bc, func(i, j int) bool { return bc[i] < bc[j] })
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
