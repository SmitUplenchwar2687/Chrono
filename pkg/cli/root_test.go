package cli

import "testing"

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd == nil {
		t.Fatal("NewRootCmd() returned nil")
	}
	if cmd.Use != "chrono" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "chrono")
	}
}
