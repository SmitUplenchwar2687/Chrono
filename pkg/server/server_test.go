package server

import "testing"

func TestNewHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("NewHub() returned nil")
	}
}
