package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

func TestCRDTStorage_NewCRDTStorage_Validation(t *testing.T) {
	_, err := NewCRDTStorage(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}

	_, err = NewCRDTStorage(&CRDTConfig{BindAddr: "127.0.0.1:0"})
	if err == nil {
		t.Fatal("expected error for missing node_id")
	}

	_, err = NewCRDTStorage(&CRDTConfig{NodeID: "n1"})
	if err == nil {
		t.Fatal("expected error for missing bind_addr")
	}
}

func TestCRDTStorage_Close_Idempotent(t *testing.T) {
	s, err := NewCRDTStorage(&CRDTConfig{
		NodeID:         "n-close",
		BindAddr:       "127.0.0.1:0",
		GossipInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCRDTStorage() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestCRDTStorage_GossipSynchronization(t *testing.T) {
	n1, n2 := newCRDTPair(t)
	defer func() {
		_ = n1.Close()
		_ = n2.Close()
	}()

	window := 30 * time.Second
	allowed, _, _, err := n1.CheckLimit(context.Background(), "user-sync", 10, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}
	n1.gossipOnce()

	if err := waitForCondition(5*time.Second, 50*time.Millisecond, func() bool {
		return hasCounterTotalAtLeast(n2, "user-sync", 1)
	}); err != nil {
		t.Fatalf("expected n2 to receive gossip state: %v", err)
	}
}

func TestCRDTStorage_GCounterMergeMaxPerNode(t *testing.T) {
	c := NewGCounter()
	c.Increment("node-a", 3)
	c.Increment("node-b", 2)

	c.Merge(map[string]int64{
		"node-a": 2, // lower than existing, ignored
		"node-b": 7, // higher than existing, applied
		"node-c": 1, // new node
	})

	snap := c.Snapshot()
	if snap["node-a"] != 3 {
		t.Fatalf("node-a = %d, want 3", snap["node-a"])
	}
	if snap["node-b"] != 7 {
		t.Fatalf("node-b = %d, want 7", snap["node-b"])
	}
	if snap["node-c"] != 1 {
		t.Fatalf("node-c = %d, want 1", snap["node-c"])
	}
}

func TestCRDTStorage_EventualConsistencyAffectsChecks(t *testing.T) {
	n1, n2 := newCRDTPair(t)
	defer func() {
		_ = n1.Close()
		_ = n2.Close()
	}()

	window := 30 * time.Second
	key := "user-ev"

	// node1 consumes the only slot for limit=1.
	allowed, _, _, err := n1.CheckLimit(context.Background(), key, 1, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}
	n1.gossipOnce()

	// Wait for node2 to converge, then it should deny for limit=1.
	if err := waitForCondition(5*time.Second, 50*time.Millisecond, func() bool {
		allowed, _, _, err := n2.CheckLimit(context.Background(), key, 1, window)
		return err == nil && !allowed
	}); err != nil {
		t.Fatalf("expected eventual deny on node2 after gossip convergence: %v", err)
	}
}

func TestCRDTStorage_MergeSnapshot_UsesVectorClockOrdering(t *testing.T) {
	s := newCRDTStorageWithoutNetwork("vc-node")

	bucketKey, resetAt := s.bucketKey("user-vc", time.Second, time.Unix(0, 0))

	s.mergeSnapshot(map[string]gossipBucket{
		bucketKey: {
			Counts:  map[string]int64{"node-a": 2},
			Version: map[string]int64{"node-a": 2},
			ResetAt: resetAt,
		},
	})
	if total := totalForBucket(s, bucketKey); total != 2 {
		t.Fatalf("total after first merge = %d, want 2", total)
	}
	if got := vectorForBucketNode(s, bucketKey, "node-a"); got != 2 {
		t.Fatalf("vector node-a after first merge = %d, want 2", got)
	}

	// Stale update should be ignored based on vector ordering.
	s.mergeSnapshot(map[string]gossipBucket{
		bucketKey: {
			Counts:  map[string]int64{"node-a": 1},
			Version: map[string]int64{"node-a": 1},
			ResetAt: resetAt,
		},
	})
	if total := totalForBucket(s, bucketKey); total != 2 {
		t.Fatalf("total after stale merge = %d, want 2", total)
	}
	if got := vectorForBucketNode(s, bucketKey, "node-a"); got != 2 {
		t.Fatalf("vector node-a after stale merge = %d, want 2", got)
	}

	// Concurrent update should merge by max component-wise.
	s.mergeSnapshot(map[string]gossipBucket{
		bucketKey: {
			Counts:  map[string]int64{"node-b": 1},
			Version: map[string]int64{"node-b": 1},
			ResetAt: resetAt,
		},
	})
	if total := totalForBucket(s, bucketKey); total != 3 {
		t.Fatalf("total after concurrent merge = %d, want 3", total)
	}
	if got := vectorForBucketNode(s, bucketKey, "node-b"); got != 1 {
		t.Fatalf("vector node-b after concurrent merge = %d, want 1", got)
	}
}

func TestCRDTStorage_GossipMetadataEnablesRemoteCleanup(t *testing.T) {
	n1, n2 := newCRDTPair(t)
	defer func() {
		_ = n1.Close()
		_ = n2.Close()
	}()

	window := 200 * time.Millisecond
	key := "user-cleanup"

	allowed, _, _, err := n1.CheckLimit(context.Background(), key, 10, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}

	n1.gossipOnce()
	if err := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		return hasCounterTotalAtLeast(n2, key, 1)
	}); err != nil {
		t.Fatalf("expected remote node to receive counter: %v", err)
	}

	// Eventually the remote bucket should disappear after reset+grace.
	if err := waitForCondition(5*time.Second, 25*time.Millisecond, func() bool {
		return !hasAnyBucketForLogicalKey(n2, key)
	}); err != nil {
		t.Fatalf("expected remote bucket cleanup after gossip metadata propagation: %v", err)
	}
}

func TestPayloadBuckets_LegacyCountersFallback(t *testing.T) {
	p := gossipPayload{
		NodeID: "legacy-node",
		Counters: map[string]map[string]int64{
			"bucket-1": {
				"node-a": 2,
			},
		},
	}

	buckets := payloadBuckets(p)
	if len(buckets) != 1 {
		t.Fatalf("len(buckets) = %d, want 1", len(buckets))
	}
	got := buckets["bucket-1"]
	if got.Counts["node-a"] != 2 {
		t.Fatalf("counts[node-a] = %d, want 2", got.Counts["node-a"])
	}
	if got.Version["node-a"] != 2 {
		t.Fatalf("version[node-a] = %d, want 2", got.Version["node-a"])
	}
}

func TestCRDTStorage_Persistence_WALRecovery_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	s := newCRDTStorageWithoutNetwork("persist-node")
	if err := s.initPersistence(&CRDTConfig{
		NodeID:           "persist-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, s)

	bucketKey, resetAt := s.bucketKey("wal-user", time.Hour, time.Now().UTC())
	changed := s.mergeSnapshot(map[string]gossipBucket{
		bucketKey: {
			Counts:  map[string]int64{"node-a": 3},
			Version: map[string]int64{"node-a": 3},
			ResetAt: resetAt,
		},
	})
	s.persistBuckets(changed)
	closePersistenceResources(t, s)

	recovered := newCRDTStorageWithoutNetwork("persist-node")
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           "persist-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("recovered initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, bucketKey); total != 3 {
		t.Fatalf("recovered total = %d, want 3", total)
	}
}

func TestCRDTStorage_Persistence_SnapshotAndDeleteRecovery_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	s := newCRDTStorageWithoutNetwork("snapshot-node")
	if err := s.initPersistence(&CRDTConfig{
		NodeID:           "snapshot-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, s)

	keepKey, keepReset := s.bucketKey("keep-user", time.Hour, time.Now().UTC())
	deleteKey, delReset := s.bucketKey("delete-user", time.Hour, time.Now().UTC())

	s.persistBuckets(s.mergeSnapshot(map[string]gossipBucket{
		keepKey: {
			Counts:  map[string]int64{"node-a": 2},
			Version: map[string]int64{"node-a": 2},
			ResetAt: keepReset,
		},
		deleteKey: {
			Counts:  map[string]int64{"node-a": 1},
			Version: map[string]int64{"node-a": 1},
			ResetAt: delReset,
		},
	}))

	s.applyDelete(deleteKey)
	s.persistDelete(deleteKey)
	if err := s.persistSnapshot(); err != nil {
		t.Fatalf("persistSnapshot() error = %v", err)
	}

	// Ensure snapshot file exists; recovery should be snapshot + WAL consistent.
	if _, err := os.Stat(filepath.Join(dir, "snapshot-node.snapshot.json")); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	closePersistenceResources(t, s)

	recovered := newCRDTStorageWithoutNetwork("snapshot-node")
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           "snapshot-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("recovered initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, keepKey); total != 2 {
		t.Fatalf("recovered keep total = %d, want 2", total)
	}
	if total := totalForBucket(recovered, deleteKey); total != 0 {
		t.Fatalf("recovered deleted total = %d, want 0", total)
	}
}

func TestCRDTStorage_Persistence_WALReplay_SkipsCorruptAndContinues_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "wal-corrupt-node"
	s := newCRDTStorageWithoutNetwork(nodeID)

	keyA, resetA := s.bucketKey("wal-a", time.Hour, time.Now().UTC())
	keyB, resetB := s.bucketKey("wal-b", time.Hour, time.Now().UTC())

	recA := walRecord{
		Op:        "upsert",
		BucketKey: keyA,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-a": 1},
			Version: map[string]int64{"node-a": 1},
			ResetAt: resetA,
		},
		At: time.Now().UTC(),
	}
	recB := walRecord{
		Op:        "upsert",
		BucketKey: keyB,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-b": 2},
			Version: map[string]int64{"node-b": 2},
			ResetAt: resetB,
		},
		At: time.Now().UTC(),
	}

	walPath := walPathForNode(dir, nodeID)
	writeWALFixture(t, walPath, []string{
		mustJSONLine(t, recA),
		"{not-json",
		mustJSONLine(t, recB),
	})

	recovered := newCRDTStorageWithoutNetwork(nodeID)
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, keyA); total != 1 {
		t.Fatalf("recovered total for keyA = %d, want 1", total)
	}
	if total := totalForBucket(recovered, keyB); total != 2 {
		t.Fatalf("recovered total for keyB = %d, want 2", total)
	}
}

func TestCRDTStorage_Persistence_WALReplay_IgnoresTruncatedTail_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "wal-tail-node"
	s := newCRDTStorageWithoutNetwork(nodeID)

	key, resetAt := s.bucketKey("wal-tail", time.Hour, time.Now().UTC())
	rec := walRecord{
		Op:        "upsert",
		BucketKey: key,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-a": 4},
			Version: map[string]int64{"node-a": 4},
			ResetAt: resetAt,
		},
		At: time.Now().UTC(),
	}

	walPath := walPathForNode(dir, nodeID)
	writeWALFixture(t, walPath, []string{
		mustJSONLine(t, rec),
		`{"op":"upsert","bucket_key":"incomplete"`,
	})

	recovered := newCRDTStorageWithoutNetwork(nodeID)
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, key); total != 4 {
		t.Fatalf("recovered total = %d, want 4", total)
	}
}

func TestCRDTStorage_Persistence_WALReplay_SkipsChecksumMismatch_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "wal-checksum-node"
	s := newCRDTStorageWithoutNetwork(nodeID)

	keyGood, resetGood := s.bucketKey("wal-good", time.Hour, time.Now().UTC())
	keyBad, resetBad := s.bucketKey("wal-bad", time.Hour, time.Now().UTC())

	recGood := walRecord{
		Op:        "upsert",
		BucketKey: keyGood,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-a": 5},
			Version: map[string]int64{"node-a": 5},
			ResetAt: resetGood,
		},
		At: time.Now().UTC(),
	}
	recBad := walRecord{
		Op:        "upsert",
		BucketKey: keyBad,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-b": 7},
			Version: map[string]int64{"node-b": 7},
			ResetAt: resetBad,
		},
		At: time.Now().UTC(),
	}

	goodLine := mustWALEnvelopeLine(t, recGood, false)
	badLine := mustWALEnvelopeLine(t, recBad, true)
	writeWALFixture(t, walPathForNode(dir, nodeID), []string{goodLine, badLine})

	recovered := newCRDTStorageWithoutNetwork(nodeID)
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, keyGood); total != 5 {
		t.Fatalf("recovered total for keyGood = %d, want 5", total)
	}
	if total := totalForBucket(recovered, keyBad); total != 0 {
		t.Fatalf("recovered total for keyBad = %d, want 0", total)
	}
}

func TestCRDTStorage_Persistence_SnapshotCompactsWAL_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "wal-compact-node"
	s := newCRDTStorageWithoutNetwork(nodeID)
	if err := s.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, s)

	keyA, resetA := s.bucketKey("compact-a", time.Hour, time.Now().UTC())
	keyB, resetB := s.bucketKey("compact-b", time.Hour, time.Now().UTC())

	s.persistBuckets(s.mergeSnapshot(map[string]gossipBucket{
		keyA: {
			Counts:  map[string]int64{"node-a": 2},
			Version: map[string]int64{"node-a": 2},
			ResetAt: resetA,
		},
	}))

	before, err := os.Stat(walPathForNode(dir, nodeID))
	if err != nil {
		t.Fatalf("stat wal before snapshot: %v", err)
	}
	if before.Size() <= 0 {
		t.Fatalf("expected WAL to have content before snapshot, got size=%d", before.Size())
	}

	if err := s.persistSnapshot(); err != nil {
		t.Fatalf("persistSnapshot() error = %v", err)
	}

	after, err := os.Stat(walPathForNode(dir, nodeID))
	if err != nil {
		t.Fatalf("stat wal after snapshot: %v", err)
	}
	if after.Size() != 0 {
		t.Fatalf("expected WAL to be compacted to empty file, got size=%d", after.Size())
	}

	// Appends must still work after compaction.
	s.persistBuckets(s.mergeSnapshot(map[string]gossipBucket{
		keyB: {
			Counts:  map[string]int64{"node-b": 3},
			Version: map[string]int64{"node-b": 3},
			ResetAt: resetB,
		},
	}))
	closePersistenceResources(t, s)

	recovered := newCRDTStorageWithoutNetwork(nodeID)
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("recovered initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, recovered)

	if total := totalForBucket(recovered, keyA); total != 2 {
		t.Fatalf("recovered total for keyA = %d, want 2", total)
	}
	if total := totalForBucket(recovered, keyB); total != 3 {
		t.Fatalf("recovered total for keyB = %d, want 3", total)
	}
}

func TestNormalizePeerURL(t *testing.T) {
	if got := normalizePeerURL("127.0.0.1:8081"); !strings.HasPrefix(got, "http://") {
		t.Fatalf("normalizePeerURL should add scheme, got %q", got)
	}
	if got := normalizePeerURL("http://127.0.0.1:8081"); got != "http://127.0.0.1:8081" {
		t.Fatalf("normalizePeerURL should keep http URL, got %q", got)
	}
}

func TestCRDTStorage_Persistence_LockPreventsSecondWriter_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "lock-node"

	s1 := newCRDTStorageWithoutNetwork(nodeID)
	if err := s1.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("first initPersistence() error = %v", err)
	}
	defer closePersistenceResources(t, s1)

	s2 := newCRDTStorageWithoutNetwork(nodeID)
	err := s2.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	})
	if err == nil {
		t.Fatal("expected lock contention error for second writer")
	}
	if !strings.Contains(err.Error(), "lock exists") {
		t.Fatalf("unexpected lock contention error: %v", err)
	}

	closePersistenceResources(t, s1)
	s3 := newCRDTStorageWithoutNetwork(nodeID)
	if err := s3.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("initPersistence() after lock release should succeed, got: %v", err)
	}
	defer closePersistenceResources(t, s3)
}

func TestCRDTStorage_Persistence_LoadSnapshot_UnsupportedVersion_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "snapshot-version-node"
	snapshotPath := snapshotPathForNode(dir, nodeID)
	payload := `{"version":999,"saved_at":"2026-01-01T00:00:00Z","buckets":{}}`
	if err := os.WriteFile(snapshotPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write snapshot fixture: %v", err)
	}

	s := newCRDTStorageWithoutNetwork(nodeID)
	err := s.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	})
	if err == nil {
		t.Fatal("expected unsupported snapshot schema version error")
	}
	if !strings.Contains(err.Error(), "unsupported snapshot schema version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCRDTStorage_Persistence_LoadWAL_UnsupportedVersion_Fails_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	nodeID := "wal-version-node"
	s := newCRDTStorageWithoutNetwork(nodeID)
	key, resetAt := s.bucketKey("wal-version", time.Hour, time.Now().UTC())
	rec := walRecord{
		Op:        "upsert",
		BucketKey: key,
		Bucket: &gossipBucket{
			Counts:  map[string]int64{"node-a": 2},
			Version: map[string]int64{"node-a": 2},
			ResetAt: resetAt,
		},
		At: time.Now().UTC(),
	}
	writeWALFixture(t, walPathForNode(dir, nodeID), []string{
		mustWALEnvelopeLineWithVersion(t, rec, 999, false),
	})

	recovered := newCRDTStorageWithoutNetwork(nodeID)
	err := recovered.initPersistence(&CRDTConfig{
		NodeID:           nodeID,
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	})
	if err == nil {
		t.Fatal("expected unsupported wal schema version error")
	}
	if !strings.Contains(err.Error(), "unsupported wal schema version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newCRDTPair(t *testing.T) (*CRDTStorage, *CRDTStorage) {
	t.Helper()

	n1, err := NewCRDTStorage(&CRDTConfig{
		NodeID:         "node-1",
		BindAddr:       "127.0.0.1:0",
		GossipInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCRDTStorage(node-1) error = %v", err)
	}

	n2, err := NewCRDTStorage(&CRDTConfig{
		NodeID:         "node-2",
		BindAddr:       "127.0.0.1:0",
		GossipInterval: 100 * time.Millisecond,
	})
	if err != nil {
		_ = n1.Close()
		t.Fatalf("NewCRDTStorage(node-2) error = %v", err)
	}

	n1.mu.Lock()
	n1.peers = []string{n2.Addr()}
	n1.mu.Unlock()

	n2.mu.Lock()
	n2.peers = []string{n1.Addr()}
	n2.mu.Unlock()

	return n1, n2
}

func waitForCondition(timeout, interval time.Duration, fn func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return nil
		}
		time.Sleep(interval)
	}
	return context.DeadlineExceeded
}

func hasCounterTotalAtLeast(s *CRDTStorage, logicalKey string, want int64) bool {
	prefix := logicalKey + "|"

	s.mu.RLock()
	keys := make([]string, 0, len(s.counters))
	counters := make(map[string]*GCounter, len(s.counters))
	for k, c := range s.counters {
		keys = append(keys, k)
		counters[k] = c
	}
	s.mu.RUnlock()

	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		counter := counters[k]
		if counter != nil && counter.Total() >= want {
			return true
		}
	}

	return false
}

func hasAnyBucketForLogicalKey(s *CRDTStorage, logicalKey string) bool {
	prefix := logicalKey + "|"
	s.mu.RLock()
	defer s.mu.RUnlock()

	for bucketKey := range s.counters {
		if strings.HasPrefix(bucketKey, prefix) {
			return true
		}
	}
	return false
}

func totalForBucket(s *CRDTStorage, bucketKey string) int64 {
	s.mu.RLock()
	counter := s.counters[bucketKey]
	s.mu.RUnlock()
	if counter == nil {
		return 0
	}
	return counter.Total()
}

func vectorForBucketNode(s *CRDTStorage, bucketKey, nodeID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vectors[bucketKey][nodeID]
}

func TestBucketKeyFormat(t *testing.T) {
	s := newCRDTStorageWithoutNetwork("fmt-node")

	k, _ := s.bucketKey("abc", time.Second, time.Unix(0, 0))
	if !strings.HasPrefix(k, "abc|") {
		t.Fatalf("unexpected bucket key format: %s", k)
	}
	if strings.Count(k, "|") != 2 {
		t.Fatalf("expected two separators in bucket key: %s", k)
	}
	if _, err := fmt.Sscanf(k, "abc|%d|%d", new(int64), new(int64)); err != nil {
		t.Fatalf("bucket key should embed numeric window metadata: %s", k)
	}
}

func newCRDTStorageWithoutNetwork(nodeID string) *CRDTStorage {
	return &CRDTStorage{
		nodeID:           nodeID,
		gossipInterval:   100 * time.Millisecond,
		snapshotInterval: time.Second,
		clock:            chronoclock.NewRealClock(),
		counters:         make(map[string]*GCounter),
		vectors:          make(map[string]map[string]int64),
		meta:             make(map[string]counterMeta),
	}
}

func walPathForNode(dir, nodeID string) string {
	return filepath.Join(dir, sanitizePathComponent(nodeID)+".wal.jsonl")
}

func snapshotPathForNode(dir, nodeID string) string {
	return filepath.Join(dir, sanitizePathComponent(nodeID)+".snapshot.json")
}

func mustJSONLine(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return string(b)
}

func writeWALFixture(t *testing.T, walPath string, lines []string) {
	t.Helper()
	data := strings.Join(lines, "\n")
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	if err := os.WriteFile(walPath, []byte(data), 0o644); err != nil {
		t.Fatalf("write wal fixture: %v", err)
	}
}

func mustWALEnvelopeLine(t *testing.T, rec walRecord, corruptChecksum bool) string {
	return mustWALEnvelopeLineWithVersion(t, rec, walSchemaVersion, corruptChecksum)
}

func mustWALEnvelopeLineWithVersion(t *testing.T, rec walRecord, version int, corruptChecksum bool) string {
	t.Helper()
	sum, err := checksumWALRecord(rec)
	if err != nil {
		t.Fatalf("checksum wal record: %v", err)
	}
	if corruptChecksum {
		sum++
	}
	return mustJSONLine(t, walEnvelope{
		Version:  version,
		Record:   rec,
		Checksum: sum,
	})
}

func closePersistenceResources(t *testing.T, s *CRDTStorage) {
	t.Helper()
	if s == nil {
		return
	}
	s.walMu.Lock()
	if s.walFile != nil {
		if err := s.walFile.Close(); err != nil {
			s.walMu.Unlock()
			t.Fatalf("close wal file: %v", err)
		}
		s.walFile = nil
	}
	s.walMu.Unlock()
	if err := s.releasePersistenceLock(); err != nil {
		t.Fatalf("release persistence lock: %v", err)
	}
}
