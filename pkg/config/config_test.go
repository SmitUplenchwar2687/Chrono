package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default() config should be valid: %v", err)
	}
}

func TestLoadFile_ParsesCRDTPersistenceFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "chrono.json")
	payload := `{
		"storage": {
			"backend": "crdt",
			"crdt": {
				"node_id": "node-a",
				"bind_addr": ":18081",
				"persist_dir": "/tmp/chrono-crdt",
				"snapshot_interval": "45s",
				"wal_sync_interval": "2s"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Storage.CRDT.PersistDir != "/tmp/chrono-crdt" {
		t.Fatalf("persist_dir = %q, want /tmp/chrono-crdt", cfg.Storage.CRDT.PersistDir)
	}
	if cfg.Storage.CRDT.SnapshotInterval != 45*time.Second {
		t.Fatalf("snapshot_interval = %v, want 45s", cfg.Storage.CRDT.SnapshotInterval)
	}
	if cfg.Storage.CRDT.WALSyncInterval != 2*time.Second {
		t.Fatalf("wal_sync_interval = %v, want 2s", cfg.Storage.CRDT.WALSyncInterval)
	}
}

func TestValidate_RejectsNegativeCRDTWALSyncInterval(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Storage.Backend = "crdt"
	cfg.Storage.CRDT.NodeID = "node-a"
	cfg.Storage.CRDT.BindAddr = ":18081"
	cfg.Storage.CRDT.WALSyncInterval = -time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for negative wal_sync_interval, got nil")
	}
	if !strings.Contains(err.Error(), "storage.crdt.wal_sync_interval") {
		t.Fatalf("Validate() error = %v, want wal_sync_interval validation error", err)
	}
}
