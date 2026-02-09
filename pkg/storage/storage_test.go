package storage

import "testing"

func TestNewStorage_DefaultsToMemory(t *testing.T) {
	s, err := NewStorage(Config{})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer s.Close()

	if _, ok := s.(*MemoryStorage); !ok {
		t.Fatalf("NewStorage() type = %T, want *MemoryStorage", s)
	}
}

func TestNewStorage_UnknownBackend(t *testing.T) {
	_, err := NewStorage(Config{Backend: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestNewStorage_PlaceholderBackends(t *testing.T) {
	_, err := NewStorage(Config{Backend: BackendRedis})
	if err == nil {
		t.Fatal("expected redis not-implemented error")
	}

	_, err = NewStorage(Config{Backend: BackendCRDT})
	if err == nil {
		t.Fatal("expected crdt not-implemented error")
	}
}
