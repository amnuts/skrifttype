package store

import (
	"context"
	"testing"
)

func TestPreferences(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, ok, err := s.GetPreference(ctx, "missing"); err != nil || ok {
		t.Fatalf("absent key: got ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	if err := s.SetPreference(ctx, "onboarding_seen", "1"); err != nil {
		t.Fatalf("SetPreference: %v", err)
	}
	v, ok, err := s.GetPreference(ctx, "onboarding_seen")
	if err != nil || !ok || v != "1" {
		t.Fatalf("get after set: v=%q ok=%v err=%v", v, ok, err)
	}

	// Upsert overwrites.
	if err := s.SetPreference(ctx, "onboarding_seen", "0"); err != nil {
		t.Fatalf("SetPreference upsert: %v", err)
	}
	v, _, _ = s.GetPreference(ctx, "onboarding_seen")
	if v != "0" {
		t.Fatalf("after upsert: v=%q want 0", v)
	}
}
