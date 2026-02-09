package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestBucketKeyFormat(t *testing.T) {
	s, err := NewCRDTStorage(&CRDTConfig{
		NodeID:         "fmt-node",
		BindAddr:       "127.0.0.1:0",
		GossipInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewCRDTStorage() error = %v", err)
	}
	defer s.Close()

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
