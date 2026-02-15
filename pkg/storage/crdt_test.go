package storage

import (
	"context"
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
	defer closeWALFile(t, s)

	bucketKey, resetAt := s.bucketKey("wal-user", time.Hour, time.Now().UTC())
	changed := s.mergeSnapshot(map[string]gossipBucket{
		bucketKey: {
			Counts:  map[string]int64{"node-a": 3},
			Version: map[string]int64{"node-a": 3},
			ResetAt: resetAt,
		},
	})
	s.persistBuckets(changed)

	recovered := newCRDTStorageWithoutNetwork("persist-node")
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           "persist-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("recovered initPersistence() error = %v", err)
	}
	defer closeWALFile(t, recovered)

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
	defer closeWALFile(t, s)

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

	recovered := newCRDTStorageWithoutNetwork("snapshot-node")
	if err := recovered.initPersistence(&CRDTConfig{
		NodeID:           "snapshot-node",
		PersistDir:       dir,
		SnapshotInterval: time.Second,
	}); err != nil {
		t.Fatalf("recovered initPersistence() error = %v", err)
	}
	defer closeWALFile(t, recovered)

	if total := totalForBucket(recovered, keepKey); total != 2 {
		t.Fatalf("recovered keep total = %d, want 2", total)
	}
	if total := totalForBucket(recovered, deleteKey); total != 0 {
		t.Fatalf("recovered deleted total = %d, want 0", total)
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

func closeWALFile(t *testing.T, s *CRDTStorage) {
	t.Helper()
	s.walMu.Lock()
	defer s.walMu.Unlock()
	if s.walFile != nil {
		if err := s.walFile.Close(); err != nil {
			t.Fatalf("close wal file: %v", err)
		}
		s.walFile = nil
	}
}
