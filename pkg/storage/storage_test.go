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
		t.Fatal("expected redis config validation error")
	}

	_, err = NewStorage(Config{
		Backend: BackendRedis,
		Redis: &RedisConfig{
			Host: "127.0.0.1",
			Port: 1,
		},
	})
	if err == nil {
		t.Fatal("expected redis connectivity/config error for invalid endpoint")
	}

	_, err = NewStorage(Config{Backend: BackendCRDT})
	if err == nil {
		t.Fatal("expected crdt config validation error")
	}

	s, err := NewStorage(Config{
		Backend: BackendCRDT,
		CRDT: &CRDTConfig{
			NodeID:   "n1",
			BindAddr: "127.0.0.1:0",
		},
	})
	if err != nil {
		t.Fatalf("expected valid crdt config to initialize, got error: %v", err)
	}
	_ = s.Close()
}
