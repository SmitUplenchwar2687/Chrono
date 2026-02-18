package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

const defaultCRDTGossipInterval = time.Second
const defaultCRDTSnapshotInterval = 30 * time.Second
const defaultCRDTWALSyncInterval = time.Second

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
// It returns allow/deny, total count after the operation, and this node's count.
func (g *GCounter) AllowBelowLimit(nodeID string, limit int) (bool, int64, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var total int64
	for _, c := range g.counts {
		total += c
	}
	if total >= int64(limit) {
		return false, total, g.counts[nodeID]
	}
	g.counts[nodeID]++
	return true, total + 1, g.counts[nodeID]
}

type counterMeta struct {
	resetAt time.Time
}

type gossipBucket struct {
	Counts  map[string]int64 `json:"counts"`
	Version map[string]int64 `json:"version,omitempty"`
	ResetAt time.Time        `json:"reset_at,omitempty"`
}

type gossipPayload struct {
	NodeID  string                  `json:"node_id"`
	Buckets map[string]gossipBucket `json:"buckets,omitempty"`
	// Legacy field for backward compatibility with older nodes.
	Counters map[string]map[string]int64 `json:"counters,omitempty"`
}

type walRecord struct {
	Op        string        `json:"op"`
	BucketKey string        `json:"bucket_key,omitempty"`
	Bucket    *gossipBucket `json:"bucket,omitempty"`
	At        time.Time     `json:"at"`
}

type walEnvelope struct {
	Record   walRecord `json:"record"`
	Checksum uint32    `json:"checksum"`
}

type snapshotRecord struct {
	SavedAt time.Time               `json:"saved_at"`
	Buckets map[string]gossipBucket `json:"buckets"`
}

// CRDTStorage is an experimental CRDT-backed storage backend.
type CRDTStorage struct {
	nodeID string
	peers  []string

	gossipInterval   time.Duration
	snapshotInterval time.Duration
	clock            chronoclock.Clock

	mu       sync.RWMutex
	counters map[string]*GCounter
	vectors  map[string]map[string]int64
	meta     map[string]counterMeta

	httpServer *http.Server
	client     *http.Client
	bindAddr   string

	persistDir      string
	persistEnable   bool
	snapshotPath    string
	walPath         string
	nextSnapshot    time.Time
	walSyncInterval time.Duration
	nextWALSync     time.Time
	walDirty        bool

	walMu   sync.Mutex
	walFile *os.File

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
	snapshotInterval := cfg.SnapshotInterval
	if snapshotInterval <= 0 {
		snapshotInterval = defaultCRDTSnapshotInterval
	}

	s := &CRDTStorage{
		nodeID:           cfg.NodeID,
		peers:            append([]string(nil), cfg.Peers...),
		gossipInterval:   interval,
		snapshotInterval: snapshotInterval,
		clock:            cfg.Clock,
		counters:         make(map[string]*GCounter),
		vectors:          make(map[string]map[string]int64),
		meta:             make(map[string]counterMeta),
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	if s.clock == nil {
		s.clock = chronoclock.NewRealClock()
	}
	if err := s.initPersistence(cfg); err != nil {
		return nil, err
	}

	log.Printf("WARNING: CRDT storage is EXPERIMENTAL - known limitations:")
	log.Printf("WARNING: - Eventual consistency (temporary drift is expected)")
	log.Printf("WARNING: - Simple HTTP polling gossip (not production-grade)")
	if !s.persistEnable {
		log.Printf("WARNING: - In-memory only (no persistence)")
	}
	log.Printf("WARNING: - Use Redis for production deployments")

	if err := s.startHTTPServer(cfg.BindAddr); err != nil {
		s.walMu.Lock()
		if s.walFile != nil {
			_ = s.walFile.Close()
			s.walFile = nil
		}
		s.walMu.Unlock()
		return nil, err
	}

	go s.gossipLoop()
	return s, nil
}

func (s *CRDTStorage) initPersistence(cfg *CRDTConfig) error {
	if cfg == nil || cfg.PersistDir == "" {
		return nil
	}

	if err := os.MkdirAll(cfg.PersistDir, 0o755); err != nil {
		return fmt.Errorf("creating crdt persist dir %s: %w", cfg.PersistDir, err)
	}

	s.persistEnable = true
	s.persistDir = cfg.PersistDir
	s.walSyncInterval = cfg.WALSyncInterval
	if s.walSyncInterval <= 0 {
		s.walSyncInterval = defaultCRDTWALSyncInterval
	}

	base := sanitizePathComponent(s.nodeID)
	if base == "" {
		base = "node"
	}
	s.snapshotPath = filepath.Join(s.persistDir, base+".snapshot.json")
	s.walPath = filepath.Join(s.persistDir, base+".wal.jsonl")

	if err := s.loadSnapshot(); err != nil {
		return err
	}
	if err := s.loadWAL(); err != nil {
		return err
	}
	s.cleanupExpired()

	walFile, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open crdt wal: %w", err)
	}
	s.walFile = walFile
	s.nextSnapshot = s.clock.Now().Add(s.snapshotInterval)
	s.nextWALSync = s.clock.Now().Add(s.walSyncInterval)
	s.walDirty = false
	return nil
}

func sanitizePathComponent(in string) string {
	var b strings.Builder
	for _, r := range in {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (s *CRDTStorage) loadSnapshot() error {
	data, err := os.ReadFile(s.snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read crdt snapshot: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var rec snapshotRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("decode crdt snapshot: %w", err)
	}
	if len(rec.Buckets) == 0 {
		return nil
	}
	_ = s.mergeSnapshot(rec.Buckets)
	return nil
}

func (s *CRDTStorage) loadWAL() error {
	f, err := os.Open(s.walPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open crdt wal: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	skipped := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		rec, ok, err := parseWALRecordFromLine(line)
		if err != nil {
			skipped++
			log.Printf("crdt wal decode warning at line %d: %v (record skipped)", lineNo, err)
			continue
		}
		if !ok {
			skipped++
			log.Printf("crdt wal decode warning at line %d: empty record (record skipped)", lineNo)
			continue
		}
		switch rec.Op {
		case "upsert":
			if rec.Bucket == nil || rec.BucketKey == "" {
				continue
			}
			_ = s.mergeSnapshot(map[string]gossipBucket{rec.BucketKey: *rec.Bucket})
		case "delete":
			if rec.BucketKey == "" {
				continue
			}
			s.applyDelete(rec.BucketKey)
		default:
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan crdt wal: %w", err)
	}
	if skipped > 0 {
		log.Printf("crdt wal replay skipped %d invalid records", skipped)
	}
	return nil
}

func parseWALRecordFromLine(line []byte) (walRecord, bool, error) {
	var env walEnvelope
	if err := json.Unmarshal(line, &env); err == nil && (env.Record.Op != "" || env.Checksum != 0) {
		expected, err := checksumWALRecord(env.Record)
		if err != nil {
			return walRecord{}, false, fmt.Errorf("wal checksum encode: %w", err)
		}
		if expected != env.Checksum {
			return walRecord{}, false, fmt.Errorf("wal checksum mismatch: got=%d want=%d", env.Checksum, expected)
		}
		return env.Record, true, nil
	}

	var rec walRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return walRecord{}, false, err
	}
	if rec.Op == "" {
		return walRecord{}, false, nil
	}
	return rec, true, nil
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

	changed := s.mergeSnapshot(payloadBuckets(payload))
	s.persistBuckets(changed)
	w.WriteHeader(http.StatusOK)
}

func payloadBuckets(p gossipPayload) map[string]gossipBucket {
	if len(p.Buckets) > 0 {
		return p.Buckets
	}
	if len(p.Counters) == 0 {
		return nil
	}

	out := make(map[string]gossipBucket, len(p.Counters))
	for bucketKey, nodeCounts := range p.Counters {
		out[bucketKey] = gossipBucket{
			Counts:  cloneVectorClock(nodeCounts),
			Version: cloneVectorClock(nodeCounts),
		}
	}
	return out
}

func (s *CRDTStorage) mergeSnapshot(snapshot map[string]gossipBucket) []string {
	if len(snapshot) == 0 {
		return nil
	}

	changedSet := make(map[string]struct{}, len(snapshot))

	s.mu.Lock()
	defer s.mu.Unlock()

	for bucketKey, incoming := range snapshot {
		if len(incoming.Counts) == 0 {
			continue
		}

		counter := s.counters[bucketKey]
		if counter == nil {
			counter = NewGCounter()
			s.counters[bucketKey] = counter
		}

		localVersion := s.vectors[bucketKey]
		incomingVersion := incoming.Version
		if len(incomingVersion) == 0 {
			incomingVersion = incoming.Counts
		}

		relation := compareVectorClock(localVersion, incomingVersion)
		if relation != localDominates && relation != localEqualsIncoming {
			counter.Merge(incoming.Counts)
			localVersion = mergeVectorClock(localVersion, incomingVersion)
			s.vectors[bucketKey] = localVersion
			changedSet[bucketKey] = struct{}{}
		}

		resetAt := incoming.ResetAt
		if resetAt.IsZero() {
			if parsed, ok := parseResetAtFromBucketKey(bucketKey); ok {
				resetAt = parsed
			}
		}
		if !resetAt.IsZero() {
			meta := s.meta[bucketKey]
			if meta.resetAt.IsZero() || resetAt.After(meta.resetAt) {
				s.meta[bucketKey] = counterMeta{resetAt: resetAt}
				changedSet[bucketKey] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(changedSet))
	for key := range changedSet {
		out = append(out, key)
	}
	return out
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
	if s.vectors[bucketKey] == nil {
		s.vectors[bucketKey] = make(map[string]int64)
	}
	meta := s.meta[bucketKey]
	if meta.resetAt.IsZero() || resetAt.After(meta.resetAt) {
		s.meta[bucketKey] = counterMeta{resetAt: resetAt}
	}
	s.mu.Unlock()

	allowed, totalAfter, localAfter := counter.AllowBelowLimit(s.nodeID, limit)
	if !allowed {
		return false, 0, resetAt, nil
	}

	s.mu.Lock()
	if s.vectors[bucketKey][s.nodeID] < localAfter {
		s.vectors[bucketKey][s.nodeID] = localAfter
	}
	s.mu.Unlock()
	s.persistBucket(bucketKey)

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
			s.maybePersistSnapshot()
			s.maybeSyncWAL()
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
		NodeID:  s.nodeID,
		Buckets: s.snapshotBuckets(),
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

func (s *CRDTStorage) snapshotBuckets() map[string]gossipBucket {
	s.mu.RLock()
	keys := make([]string, 0, len(s.counters))
	for k := range s.counters {
		keys = append(keys, k)
	}
	counters := make(map[string]*GCounter, len(keys))
	vectors := make(map[string]map[string]int64, len(keys))
	resetTimes := make(map[string]time.Time, len(keys))
	for _, k := range keys {
		counters[k] = s.counters[k]
		vectors[k] = cloneVectorClock(s.vectors[k])
		if meta, ok := s.meta[k]; ok {
			resetTimes[k] = meta.resetAt
		}
	}
	s.mu.RUnlock()

	out := make(map[string]gossipBucket, len(counters))
	for k, counter := range counters {
		counts := counter.Snapshot()
		version := vectors[k]
		if len(version) == 0 {
			version = cloneVectorClock(counts)
		}

		resetAt := resetTimes[k]
		if resetAt.IsZero() {
			if parsed, ok := parseResetAtFromBucketKey(k); ok {
				resetAt = parsed
			}
		}

		out[k] = gossipBucket{
			Counts:  counts,
			Version: version,
			ResetAt: resetAt,
		}
	}
	return out
}

func cloneVectorClock(src map[string]int64) map[string]int64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int64, len(src))
	for nodeID, v := range src {
		out[nodeID] = v
	}
	return out
}

func mergeVectorClock(local, incoming map[string]int64) map[string]int64 {
	if len(local) == 0 && len(incoming) == 0 {
		return nil
	}
	out := cloneVectorClock(local)
	if out == nil {
		out = make(map[string]int64, len(incoming))
	}
	for nodeID, incomingV := range incoming {
		if incomingV > out[nodeID] {
			out[nodeID] = incomingV
		}
	}
	return out
}

type vectorClockRelation int

const (
	localEqualsIncoming vectorClockRelation = iota
	localDominates
	localIsDominated
	localConcurrent
)

func compareVectorClock(local, incoming map[string]int64) vectorClockRelation {
	localGEIncoming := true
	incomingGELocal := true

	for nodeID, localV := range local {
		incomingV := incoming[nodeID]
		if localV < incomingV {
			localGEIncoming = false
		}
		if incomingV < localV {
			incomingGELocal = false
		}
	}
	for nodeID, incomingV := range incoming {
		localV := local[nodeID]
		if localV < incomingV {
			localGEIncoming = false
		}
		if incomingV < localV {
			incomingGELocal = false
		}
	}

	switch {
	case localGEIncoming && incomingGELocal:
		return localEqualsIncoming
	case localGEIncoming:
		return localDominates
	case incomingGELocal:
		return localIsDominated
	default:
		return localConcurrent
	}
}

func parseResetAtFromBucketKey(bucketKey string) (time.Time, bool) {
	last := strings.LastIndex(bucketKey, "|")
	if last < 0 || last == len(bucketKey)-1 {
		return time.Time{}, false
	}
	windowStr := bucketKey[last+1:]
	prefix := bucketKey[:last]

	secondLast := strings.LastIndex(prefix, "|")
	if secondLast < 0 || secondLast == len(prefix)-1 {
		return time.Time{}, false
	}
	windowIDStr := prefix[secondLast+1:]

	windowNanos, err := strconv.ParseInt(windowStr, 10, 64)
	if err != nil || windowNanos <= 0 {
		return time.Time{}, false
	}
	windowID, err := strconv.ParseInt(windowIDStr, 10, 64)
	if err != nil || windowID < 0 {
		return time.Time{}, false
	}

	return time.Unix(0, (windowID+1)*windowNanos), true
}

func (s *CRDTStorage) maybePersistSnapshot() {
	if !s.persistEnable || s.snapshotInterval <= 0 {
		return
	}

	now := s.clock.Now()
	if !s.nextSnapshot.IsZero() && now.Before(s.nextSnapshot) {
		return
	}
	if err := s.persistSnapshot(); err != nil {
		log.Printf("crdt snapshot persist error: %v", err)
	}
	s.nextSnapshot = now.Add(s.snapshotInterval)
}

func (s *CRDTStorage) maybeSyncWAL() {
	if !s.persistEnable || s.walSyncInterval <= 0 {
		return
	}

	now := s.clock.Now()

	s.walMu.Lock()
	defer s.walMu.Unlock()
	if s.walFile == nil || !s.walDirty {
		return
	}
	if !s.nextWALSync.IsZero() && now.Before(s.nextWALSync) {
		return
	}
	if err := s.walFile.Sync(); err != nil {
		log.Printf("crdt wal sync error: %v", err)
		return
	}
	s.walDirty = false
	s.nextWALSync = now.Add(s.walSyncInterval)
}

func (s *CRDTStorage) persistSnapshot() error {
	if !s.persistEnable {
		return nil
	}

	s.walMu.Lock()
	defer s.walMu.Unlock()

	rec := snapshotRecord{
		SavedAt: time.Now().UTC(),
		Buckets: s.snapshotBuckets(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal crdt snapshot: %w", err)
	}

	tmpPath := s.snapshotPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write crdt snapshot temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.snapshotPath); err != nil {
		return fmt.Errorf("rename crdt snapshot: %w", err)
	}

	if err := s.rotateWALLocked(); err != nil {
		return err
	}
	return nil
}

func (s *CRDTStorage) rotateWALLocked() error {
	// caller must hold walMu
	if !s.persistEnable {
		return nil
	}

	if s.walFile != nil {
		if err := s.walFile.Sync(); err != nil {
			return fmt.Errorf("sync crdt wal before rotate: %w", err)
		}
		if err := s.walFile.Close(); err != nil {
			return fmt.Errorf("close crdt wal before rotate: %w", err)
		}
		s.walFile = nil
	}

	walFile, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("reopen crdt wal after rotate: %w", err)
	}
	s.walFile = walFile
	s.walDirty = false
	s.nextWALSync = s.clock.Now().Add(s.walSyncInterval)
	return nil
}

func (s *CRDTStorage) persistBuckets(bucketKeys []string) {
	if len(bucketKeys) == 0 {
		return
	}
	for _, bucketKey := range bucketKeys {
		s.persistBucket(bucketKey)
	}
}

func (s *CRDTStorage) persistBucket(bucketKey string) {
	if !s.persistEnable || bucketKey == "" {
		return
	}

	bucket, ok := s.snapshotBucket(bucketKey)
	if !ok {
		return
	}

	rec := walRecord{
		Op:        "upsert",
		BucketKey: bucketKey,
		Bucket:    &bucket,
		At:        time.Now().UTC(),
	}
	if err := s.appendWAL(rec); err != nil {
		log.Printf("crdt wal append error for bucket %s: %v", bucketKey, err)
	}
}

func (s *CRDTStorage) snapshotBucket(bucketKey string) (gossipBucket, bool) {
	s.mu.RLock()
	counter := s.counters[bucketKey]
	vector := cloneVectorClock(s.vectors[bucketKey])
	meta, hasMeta := s.meta[bucketKey]
	s.mu.RUnlock()
	if counter == nil {
		return gossipBucket{}, false
	}

	counts := counter.Snapshot()
	if len(counts) == 0 {
		return gossipBucket{}, false
	}
	if len(vector) == 0 {
		vector = cloneVectorClock(counts)
	}

	resetAt := time.Time{}
	if hasMeta {
		resetAt = meta.resetAt
	} else if parsed, ok := parseResetAtFromBucketKey(bucketKey); ok {
		resetAt = parsed
	}

	return gossipBucket{
		Counts:  counts,
		Version: vector,
		ResetAt: resetAt,
	}, true
}

func (s *CRDTStorage) persistDelete(bucketKey string) {
	if !s.persistEnable || bucketKey == "" {
		return
	}
	rec := walRecord{
		Op:        "delete",
		BucketKey: bucketKey,
		At:        time.Now().UTC(),
	}
	if err := s.appendWAL(rec); err != nil {
		log.Printf("crdt wal append delete error for bucket %s: %v", bucketKey, err)
	}
}

func (s *CRDTStorage) appendWAL(rec walRecord) error {
	if !s.persistEnable || s.walFile == nil {
		return nil
	}
	sum, err := checksumWALRecord(rec)
	if err != nil {
		return fmt.Errorf("checksum crdt wal record: %w", err)
	}

	data, err := json.Marshal(walEnvelope{
		Record:   rec,
		Checksum: sum,
	})
	if err != nil {
		return fmt.Errorf("marshal crdt wal envelope: %w", err)
	}

	s.walMu.Lock()
	defer s.walMu.Unlock()
	if _, err := s.walFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write crdt wal record: %w", err)
	}
	s.walDirty = true
	if s.walSyncInterval <= 0 {
		if err := s.walFile.Sync(); err != nil {
			return fmt.Errorf("sync crdt wal: %w", err)
		}
		s.walDirty = false
	} else if s.nextWALSync.IsZero() {
		s.nextWALSync = s.clock.Now().Add(s.walSyncInterval)
	}
	return nil
}

func checksumWALRecord(rec walRecord) (uint32, error) {
	b, err := json.Marshal(rec)
	if err != nil {
		return 0, err
	}
	return crc32.ChecksumIEEE(b), nil
}

func (s *CRDTStorage) applyDelete(bucketKey string) {
	s.mu.Lock()
	delete(s.counters, bucketKey)
	delete(s.vectors, bucketKey)
	delete(s.meta, bucketKey)
	s.mu.Unlock()
}

func (s *CRDTStorage) cleanupExpired() {
	now := s.clock.Now()
	var deleted []string

	s.mu.Lock()
	for k, meta := range s.meta {
		// Keep counters slightly beyond window end to allow gossip convergence.
		if now.After(meta.resetAt.Add(2 * s.gossipInterval)) {
			delete(s.meta, k)
			delete(s.counters, k)
			delete(s.vectors, k)
			deleted = append(deleted, k)
		}
	}
	s.mu.Unlock()

	for _, bucketKey := range deleted {
		s.persistDelete(bucketKey)
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
		if err := s.persistSnapshot(); err != nil && retErr == nil {
			retErr = err
		}
		s.walMu.Lock()
		if s.walFile != nil {
			if err := s.walFile.Sync(); err != nil && retErr == nil {
				retErr = err
			}
			if err := s.walFile.Close(); err != nil && retErr == nil {
				retErr = err
			}
			s.walFile = nil
		}
		s.walMu.Unlock()
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
