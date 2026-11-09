package store_test

import (
	"context"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestWatchedRoots_AddAndList(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	r1, err := s.AddWatchedRoot(ctx, `D:\fonts\client-a`, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot 1: %v", err)
	}
	if r1.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if r1.Path != `D:\fonts\client-a` {
		t.Errorf("Path: got %q, want %q", r1.Path, `D:\fonts\client-a`)
	}
	if !r1.Recursive {
		t.Error("Recursive: expected true")
	}
	if !r1.Enabled {
		t.Error("Enabled: expected true (default)")
	}

	r2, err := s.AddWatchedRoot(ctx, `D:\fonts\client-b`, false)
	if err != nil {
		t.Fatalf("AddWatchedRoot 2: %v", err)
	}
	if r2.ID == r1.ID {
		t.Errorf("expected distinct IDs, got %d twice", r1.ID)
	}

	list, err := s.ListWatchedRoots(ctx)
	if err != nil {
		t.Fatalf("ListWatchedRoots: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(list))
	}
}

func TestWatchedRoots_AddDuplicatePathFails(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	if _, err := s.AddWatchedRoot(ctx, `D:\fonts\one`, true); err != nil {
		t.Fatalf("first AddWatchedRoot: %v", err)
	}
	if _, err := s.AddWatchedRoot(ctx, `D:\fonts\one`, true); err == nil {
		t.Fatal("expected error inserting duplicate path")
	}
}

func TestWatchedRoots_SetEnabled(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	r, err := s.AddWatchedRoot(ctx, `D:\fonts\test`, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}
	if err := s.SetWatchedRootEnabled(ctx, r.ID, false); err != nil {
		t.Fatalf("SetWatchedRootEnabled false: %v", err)
	}

	enabled, err := s.ListEnabledWatchedRoots(ctx)
	if err != nil {
		t.Fatalf("ListEnabledWatchedRoots: %v", err)
	}
	if len(enabled) != 0 {
		t.Errorf("expected 0 enabled roots after disabling, got %d", len(enabled))
	}

	if err := s.SetWatchedRootEnabled(ctx, r.ID, true); err != nil {
		t.Fatalf("SetWatchedRootEnabled true: %v", err)
	}
	enabled, err = s.ListEnabledWatchedRoots(ctx)
	if err != nil {
		t.Fatalf("ListEnabledWatchedRoots: %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled root after re-enabling, got %d", len(enabled))
	}
}

func TestWatchedRoots_Remove(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	r, err := s.AddWatchedRoot(ctx, `D:\fonts\to-remove`, true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}
	if err := s.RemoveWatchedRoot(ctx, r.ID); err != nil {
		t.Fatalf("RemoveWatchedRoot: %v", err)
	}
	list, err := s.ListWatchedRoots(ctx)
	if err != nil {
		t.Fatalf("ListWatchedRoots: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 roots after removal, got %d", len(list))
	}
}
