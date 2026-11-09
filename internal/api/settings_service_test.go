package api

import (
	"context"
	"testing"

	"github.com/amnuts/skrifttype/internal/store"
)

func newTestService(t *testing.T) *SettingsService {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &SettingsService{store: st}
}

func TestSettingsServiceWatchedRoots(t *testing.T) {
	s := newTestService(t)
	added, err := s.AddWatchedRoot("C:\\Fonts", true)
	if err != nil {
		t.Fatalf("AddWatchedRoot: %v", err)
	}
	if added.Path != "C:\\Fonts" || !added.Recursive || !added.Enabled || added.ID == 0 {
		t.Fatalf("unexpected DTO: %+v", added)
	}
	list, err := s.ListWatchedRoots()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListWatchedRoots: list=%v err=%v", list, err)
	}
	if err := s.SetWatchedRootEnabled(added.ID, false); err != nil {
		t.Fatalf("SetWatchedRootEnabled: %v", err)
	}
	list, _ = s.ListWatchedRoots()
	if list[0].Enabled {
		t.Fatalf("expected disabled after SetWatchedRootEnabled(false)")
	}
	if err := s.RemoveWatchedRoot(added.ID); err != nil {
		t.Fatalf("RemoveWatchedRoot: %v", err)
	}
	list, _ = s.ListWatchedRoots()
	if len(list) != 0 {
		t.Fatalf("expected empty after remove, got %v", list)
	}
}

func TestSettingsServicePreferences(t *testing.T) {
	s := newTestService(t)
	if v, err := s.GetPreference("onboarding_seen"); err != nil || v != "" {
		t.Fatalf("absent pref: v=%q err=%v", v, err)
	}
	if err := s.SetPreference("onboarding_seen", "1"); err != nil {
		t.Fatalf("SetPreference: %v", err)
	}
	if v, _ := s.GetPreference("onboarding_seen"); v != "1" {
		t.Fatalf("get after set: %q", v)
	}
}
