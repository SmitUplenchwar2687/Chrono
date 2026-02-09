package storage

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

const defaultCleanupInterval = time.Minute

// MemoryConfig configures the in-memory backend.
type MemoryConfig struct {
	CleanupInterval time.Duration     `json:"cleanup_interval" yaml:"cleanup_interval"`
	Algorithm       string            `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`
	Burst           int               `json:"burst,omitempty" yaml:"burst,omitempty"`
	Clock           chronoclock.Clock `json:"-" yaml:"-"`
}

// MemoryStorage is an in-memory implementation of Storage.
// It supports fixed window, sliding window, and token bucket modes.
type MemoryStorage struct {
	mu sync.RWMutex

	clock           chronoclock.Clock
	algorithm       string
	cleanupInterval time.Duration
	burst           int

	fixed   map[string]fixedWindowState
	sliding map[string]slidingWindowState
	token   map[string]tokenBucketState

	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

type fixedWindowState struct {
	count    int
	windowID int64
	window   time.Duration
	lastSeen time.Time
}

type slidingWindowState struct {
	entries  []time.Time
	window   time.Duration
	lastSeen time.Time
}

type tokenBucketState struct {
	tokens   float64
	lastFill time.Time
	lastSeen time.Time
	rate     float64
	capacity int
	window   time.Duration
}

// NewMemoryStorage constructs a memory-backed Storage.
func NewMemoryStorage(cfg *MemoryConfig) (*MemoryStorage, error) {
	settings := defaultMemoryConfig()
	if cfg != nil {
		if cfg.CleanupInterval > 0 {
			settings.CleanupInterval = cfg.CleanupInterval
		}
		if cfg.Algorithm != "" {
			settings.Algorithm = cfg.Algorithm
		}
		if cfg.Burst > 0 {
			settings.Burst = cfg.Burst
		}
		if cfg.Clock != nil {
			settings.Clock = cfg.Clock
		}
	}

	if settings.CleanupInterval <= 0 {
		return nil, fmt.Errorf("cleanup_interval must be positive, got %s", settings.CleanupInterval)
	}
	switch settings.Algorithm {
	case AlgorithmTokenBucket, AlgorithmSlidingWindow, AlgorithmFixedWindow:
	default:
		return nil, fmt.Errorf("unknown memory algorithm %q", settings.Algorithm)
	}

	s := &MemoryStorage{
		clock:           settings.Clock,
		algorithm:       settings.Algorithm,
		cleanupInterval: settings.CleanupInterval,
		burst:           settings.Burst,
		fixed:           make(map[string]fixedWindowState),
		sliding:         make(map[string]slidingWindowState),
		token:           make(map[string]tokenBucketState),
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
	go s.cleanupLoop()

	return s, nil
}

func defaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		CleanupInterval: defaultCleanupInterval,
		Algorithm:       AlgorithmFixedWindow,
		Clock:           chronoclock.NewRealClock(),
	}
}

// CheckLimit checks and updates rate-limit state for key in one critical section.
func (s *MemoryStorage) CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	switch s.algorithm {
	case AlgorithmTokenBucket:
		return s.checkTokenBucket(now, key, limit, window)
	case AlgorithmSlidingWindow:
		return s.checkSlidingWindow(now, key, limit, window)
	case AlgorithmFixedWindow:
		return s.checkFixedWindow(now, key, limit, window)
	default:
		return false, 0, time.Time{}, fmt.Errorf("unsupported algorithm %q", s.algorithm)
	}
}

func (s *MemoryStorage) checkFixedWindow(now time.Time, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	windowID := now.UnixNano() / int64(window)
	windowStart := time.Unix(0, windowID*int64(window))
	resetAt := windowStart.Add(window)

	state, ok := s.fixed[key]
	if !ok || state.windowID != windowID || state.window != window {
		state = fixedWindowState{
			count:    0,
			windowID: windowID,
			window:   window,
		}
	}

	state.lastSeen = now
	if state.count < limit {
		state.count++
		s.fixed[key] = state
		return true, limit - state.count, resetAt, nil
	}

	s.fixed[key] = state
	return false, 0, resetAt, nil
}

func (s *MemoryStorage) checkSlidingWindow(now time.Time, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	state := s.sliding[key]
	if state.window != window {
		state.window = window
	}

	windowStart := now.Add(-window)
	pruned := state.entries[:0]
	for _, ts := range state.entries {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}
	state.entries = pruned
	state.lastSeen = now

	count := len(state.entries)
	var resetAt time.Time
	if count > 0 {
		resetAt = state.entries[0].Add(window)
	} else {
		resetAt = now.Add(window)
	}

	if count < limit {
		state.entries = append(state.entries, now)
		s.sliding[key] = state
		return true, limit - count - 1, resetAt, nil
	}

	s.sliding[key] = state
	return false, 0, resetAt, nil
}

func (s *MemoryStorage) checkTokenBucket(now time.Time, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	capacity := limit
	if s.burst > 0 {
		capacity = s.burst
	}

	state, ok := s.token[key]
	if !ok || state.window != window || state.capacity != capacity {
		state = tokenBucketState{
			tokens:   float64(capacity),
			lastFill: now,
			lastSeen: now,
			rate:     float64(limit) / window.Seconds(),
			capacity: capacity,
			window:   window,
		}
	}

	elapsed := now.Sub(state.lastFill).Seconds()
	state.tokens += elapsed * state.rate
	if state.tokens > float64(state.capacity) {
		state.tokens = float64(state.capacity)
	}
	state.lastFill = now
	state.lastSeen = now

	if state.tokens >= 1.0 {
		state.tokens -= 1.0
		s.token[key] = state

		deficit := float64(state.capacity) - state.tokens
		resetAt := now
		if deficit > 0 {
			resetAt = now.Add(time.Duration(deficit / state.rate * float64(time.Second)))
		}

		return true, int(math.Floor(state.tokens)), resetAt, nil
	}

	s.token[key] = state
	needed := 1.0 - state.tokens
	resetAt := now.Add(time.Duration(needed / state.rate * float64(time.Second)))
	return false, 0, resetAt, nil
}

func (s *MemoryStorage) cleanupLoop() {
	ticker := time.NewTicker(s.cleanupInterval)
	defer func() {
		ticker.Stop()
		close(s.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *MemoryStorage) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	switch s.algorithm {
	case AlgorithmFixedWindow:
		for key, state := range s.fixed {
			windowStart := time.Unix(0, state.windowID*int64(state.window))
			if !now.Before(windowStart.Add(state.window)) {
				delete(s.fixed, key)
			}
		}
	case AlgorithmSlidingWindow:
		for key, state := range s.sliding {
			windowStart := now.Add(-state.window)
			pruned := state.entries[:0]
			for _, ts := range state.entries {
				if ts.After(windowStart) {
					pruned = append(pruned, ts)
				}
			}
			state.entries = pruned
			if len(state.entries) == 0 {
				delete(s.sliding, key)
				continue
			}
			s.sliding[key] = state
		}
	case AlgorithmTokenBucket:
		for key, state := range s.token {
			if now.Sub(state.lastSeen) > state.window && state.tokens >= float64(state.capacity) {
				delete(s.token, key)
			}
		}
	}
}

// Close stops background cleanup and releases resources.
func (s *MemoryStorage) Close() error {
	s.closeOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
	})
	return nil
}
