package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

const defaultCRDTGossipInterval = time.Second

// GCounter is a grow-only counter CRDT.
// It keeps per-node counts and merges by taking max per node.
type GCounter struct {
	mu     sync.RWMutex
	counts map[string]int64
}

// NewGCounter creates an empty grow-only counter.
func NewGCounter() *GCounter {
	return &GCounter{
		counts: make(map[string]int64),
	}
}

// Increment increases this node's count by delta.
func (g *GCounter) Increment(nodeID string, delta int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counts[nodeID] += delta
}

// Merge merges another counter state by taking max(nodeCount).
func (g *GCounter) Merge(other map[string]int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for nodeID, c := range other {
		if c > g.counts[nodeID] {
			g.counts[nodeID] = c
		}
	}
}

// Total returns sum of all node counts.
func (g *GCounter) Total() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var total int64
	for _, c := range g.counts {
		total += c
	}
	return total
}

// Snapshot returns a deep copy of counts.
func (g *GCounter) Snapshot() map[string]int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string]int64, len(g.counts))
	for nodeID, c := range g.counts {
		out[nodeID] = c
	}
	return out
}

// AllowBelowLimit checks total and increments atomically if still below limit.
func (g *GCounter) AllowBelowLimit(nodeID string, limit int) (bool, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var total int64
	for _, c := range g.counts {
		total += c
	}
	if total >= int64(limit) {
		return false, total
	}
	g.counts[nodeID]++
	return true, total + 1
}

type counterMeta struct {
	resetAt time.Time
}

type gossipPayload struct {
	NodeID   string                      `json:"node_id"`
	Counters map[string]map[string]int64 `json:"counters"`
}

// CRDTStorage is an experimental CRDT-backed storage backend.
type CRDTStorage struct {
	nodeID string
	peers  []string

	gossipInterval time.Duration
	clock          chronoclock.Clock

	mu       sync.RWMutex
	counters map[string]*GCounter
	meta     map[string]counterMeta

	httpServer *http.Server
	client     *http.Client
	bindAddr   string

	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

// NewCRDTStorage creates an experimental CRDT storage backend.
func NewCRDTStorage(cfg *CRDTConfig) (*CRDTStorage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("crdt config is required")
	}
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("crdt node_id is required")
	}
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("crdt bind_addr is required")
	}

	interval := cfg.GossipInterval
	if interval <= 0 {
		interval = defaultCRDTGossipInterval
	}

	s := &CRDTStorage{
		nodeID:         cfg.NodeID,
		peers:          append([]string(nil), cfg.Peers...),
		gossipInterval: interval,
		clock:          cfg.Clock,
		counters:       make(map[string]*GCounter),
		meta:           make(map[string]counterMeta),
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	if s.clock == nil {
		s.clock = chronoclock.NewRealClock()
	}

	log.Printf("WARNING: CRDT storage is EXPERIMENTAL - known limitations:")
	log.Printf("WARNING: - No vector clocks (may have counter drift)")
	log.Printf("WARNING: - Simple HTTP polling gossip (not production-grade)")
	log.Printf("WARNING: - In-memory only (no persistence)")
	log.Printf("WARNING: - Use Redis for production deployments")

	if err := s.startHTTPServer(cfg.BindAddr); err != nil {
		return nil, err
	}

	go s.gossipLoop()
	return s, nil
}

func (s *CRDTStorage) startHTTPServer(bindAddr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/gossip", s.handleGossip)

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("starting crdt listener on %s: %w", bindAddr, err)
	}

	s.bindAddr = ln.Addr().String()
	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("crdt gossip server error: %v", err)
		}
	}()

	return nil
}

func (s *CRDTStorage) handleGossip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var payload gossipPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid gossip payload", http.StatusBadRequest)
		return
	}
	if payload.NodeID == "" {
		http.Error(w, "missing node_id", http.StatusBadRequest)
		return
	}

	s.mergeSnapshot(payload.Counters)
	w.WriteHeader(http.StatusOK)
}

func (s *CRDTStorage) mergeSnapshot(snapshot map[string]map[string]int64) {
	if len(snapshot) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for bucketKey, nodeCounts := range snapshot {
		counter := s.counters[bucketKey]
		if counter == nil {
			counter = NewGCounter()
			s.counters[bucketKey] = counter
		}
		counter.Merge(nodeCounts)
	}
}

// CheckLimit checks if request is allowed based on CRDT counter in current window bucket.
func (s *CRDTStorage) CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	select {
	case <-ctx.Done():
		return false, 0, time.Time{}, ctx.Err()
	default:
	}

	if key == "" {
		return false, 0, time.Time{}, fmt.Errorf("key is required")
	}
	if limit <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("limit must be positive, got %d", limit)
	}
	if window <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("window must be positive, got %s", window)
	}

	now := s.clock.Now()
	bucketKey, resetAt := s.bucketKey(key, window, now)

	s.mu.Lock()
	counter := s.counters[bucketKey]
	if counter == nil {
		counter = NewGCounter()
		s.counters[bucketKey] = counter
	}
	s.meta[bucketKey] = counterMeta{resetAt: resetAt}
	s.mu.Unlock()

	allowed, totalAfter := counter.AllowBelowLimit(s.nodeID, limit)
	if !allowed {
		return false, 0, resetAt, nil
	}

	remaining := limit - int(totalAfter)
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining, resetAt, nil
}

func (s *CRDTStorage) bucketKey(key string, window time.Duration, at time.Time) (string, time.Time) {
	windowID := at.UnixNano() / int64(window)
	resetAt := time.Unix(0, (windowID+1)*int64(window))
	// Include window size in bucket to avoid collisions across different windows.
	return fmt.Sprintf("%s|%d|%d", key, windowID, int64(window)), resetAt
}

func (s *CRDTStorage) gossipLoop() {
	ticker := time.NewTicker(s.gossipInterval)
	defer func() {
		ticker.Stop()
		close(s.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			s.gossipOnce()
			s.cleanupExpired()
		case <-s.stopCh:
			return
		}
	}
}

func (s *CRDTStorage) gossipOnce() {
	peers := s.snapshotPeers()
	if len(peers) == 0 {
		return
	}

	payload := gossipPayload{
		NodeID:   s.nodeID,
		Counters: s.snapshotCounters(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("crdt gossip marshal error: %v", err)
		return
	}

	for _, peer := range peers {
		url := normalizePeerURL(peer) + "/gossip"
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			log.Printf("crdt gossip request build error for %s: %v", peer, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			log.Printf("crdt gossip post to %s failed: %v", peer, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func (s *CRDTStorage) snapshotPeers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, len(s.peers))
	copy(out, s.peers)
	return out
}

func (s *CRDTStorage) snapshotCounters() map[string]map[string]int64 {
	s.mu.RLock()
	keys := make([]string, 0, len(s.counters))
	for k := range s.counters {
		keys = append(keys, k)
	}
	counters := make(map[string]*GCounter, len(keys))
	for _, k := range keys {
		counters[k] = s.counters[k]
	}
	s.mu.RUnlock()

	out := make(map[string]map[string]int64, len(counters))
	for k, counter := range counters {
		out[k] = counter.Snapshot()
	}
	return out
}

func (s *CRDTStorage) cleanupExpired() {
	now := s.clock.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, meta := range s.meta {
		// Keep counters slightly beyond window end to allow gossip convergence.
		if now.After(meta.resetAt.Add(2 * s.gossipInterval)) {
			delete(s.meta, k)
			delete(s.counters, k)
		}
	}
}

func normalizePeerURL(peer string) string {
	if strings.HasPrefix(peer, "http://") || strings.HasPrefix(peer, "https://") {
		return peer
	}
	return "http://" + peer
}

// Addr returns the effective listening address.
// Useful in tests when bind_addr uses :0.
func (s *CRDTStorage) Addr() string {
	return s.bindAddr
}

// Close gracefully stops gossip worker and HTTP server. It is idempotent.
func (s *CRDTStorage) Close() error {
	var retErr error
	s.closeOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
		if s.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.httpServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				retErr = err
			}
		}
	})
	return retErr
}
