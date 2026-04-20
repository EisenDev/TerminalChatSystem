package auth

import "testing"

func TestNormalizeHandle(t *testing.T) {
	got, err := NormalizeHandle(" Alice_01 ")
	if err != nil {
		t.Fatalf("NormalizeHandle() error = %v", err)
	}
	if got != "alice_01" {
		t.Fatalf("expected alice_01, got %q", got)
	}
}

func TestNormalizeHandleRejectsInvalid(t *testing.T) {
	if _, err := NormalizeHandle("a!"); err == nil {
		t.Fatal("expected error for invalid handle")
	}
}
