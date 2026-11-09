package store_test

import (
	"context"
	"testing"
)

func TestCollections_Create_Get_Rename_Delete(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	c, err := s.CreateCollection(ctx, "Web UI")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if c.Name != "Web UI" {
		t.Errorf("Name: got %q, want Web UI", c.Name)
	}
	if c.CreatedAt == 0 {
		t.Error("expected non-zero CreatedAt")
	}

	got, err := s.GetCollection(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.Name != "Web UI" {
		t.Errorf("Name (get): got %q", got.Name)
	}

	if err := s.RenameCollection(ctx, c.ID, "Web Brand"); err != nil {
		t.Fatalf("RenameCollection: %v", err)
	}
	got, err = s.GetCollection(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCollection (post-rename): %v", err)
	}
	if got.Name != "Web Brand" {
		t.Errorf("Name (renamed): got %q", got.Name)
	}

	if err := s.DeleteCollection(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	if _, err := s.GetCollection(ctx, c.ID); err == nil {
		t.Fatal("expected error fetching deleted collection")
	}
}

func TestCollections_DuplicateNameFails(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	if _, err := s.CreateCollection(ctx, "Print"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := s.CreateCollection(ctx, "Print"); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestCollections_ListCollections(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	for _, name := range []string{"A", "B", "C"} {
		if _, err := s.CreateCollection(ctx, name); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	all, err := s.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 collections, got %d", len(all))
	}
}

func TestCollections_Membership(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	c, err := s.CreateCollection(ctx, "Set X")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	f1id, err := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\one.ttf`))
	if err != nil {
		t.Fatalf("upsert f1: %v", err)
	}
	f2id, err := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\two.ttf`))
	if err != nil {
		t.Fatalf("upsert f2: %v", err)
	}

	if err := s.AddFontToCollection(ctx, c.ID, f1id); err != nil {
		t.Fatalf("AddFontToCollection f1: %v", err)
	}
	if err := s.AddFontToCollection(ctx, c.ID, f2id); err != nil {
		t.Fatalf("AddFontToCollection f2: %v", err)
	}
	// Idempotent: re-adding is not an error.
	if err := s.AddFontToCollection(ctx, c.ID, f1id); err != nil {
		t.Fatalf("AddFontToCollection f1 (re-add): %v", err)
	}

	members, err := s.ListFontsInCollection(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListFontsInCollection: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	collsForF1, err := s.ListCollectionsForFont(ctx, f1id)
	if err != nil {
		t.Fatalf("ListCollectionsForFont: %v", err)
	}
	if len(collsForF1) != 1 || collsForF1[0] != c.ID {
		t.Errorf("ListCollectionsForFont: got %v, want [%d]", collsForF1, c.ID)
	}

	if err := s.RemoveFontFromCollection(ctx, c.ID, f1id); err != nil {
		t.Fatalf("RemoveFontFromCollection: %v", err)
	}
	// Idempotent: removing a non-member is not an error.
	if err := s.RemoveFontFromCollection(ctx, c.ID, f1id); err != nil {
		t.Fatalf("RemoveFontFromCollection (already removed): %v", err)
	}

	members, err = s.ListFontsInCollection(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListFontsInCollection (after remove): %v", err)
	}
	if len(members) != 1 || members[0] != f2id {
		t.Errorf("expected only f2 to remain; got %v", members)
	}
}

func TestCollections_DeleteCascadesMembers(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	c, err := s.CreateCollection(ctx, "Temp")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	fid, err := s.UpsertFont(ctx, sampleLibraryFont(`D:\fonts\member.ttf`))
	if err != nil {
		t.Fatalf("UpsertFont: %v", err)
	}
	if err := s.AddFontToCollection(ctx, c.ID, fid); err != nil {
		t.Fatalf("AddFontToCollection: %v", err)
	}
	if err := s.DeleteCollection(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}

	// Membership row must be gone (FK ON DELETE CASCADE).
	colls, err := s.ListCollectionsForFont(ctx, fid)
	if err != nil {
		t.Fatalf("ListCollectionsForFont: %v", err)
	}
	if len(colls) != 0 {
		t.Errorf("expected 0 collections after parent delete, got %v", colls)
	}
}
