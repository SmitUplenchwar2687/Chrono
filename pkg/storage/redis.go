package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisPoolSize    = 20
	defaultRedisMaxRetries  = 3
	defaultRedisDialTimeout = 5 * time.Second

	redisRateLimitPrefix = "chrono:rl:"
)

var redisSlidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
local count = redis.call('ZCARD', key)

local allowed = 0
local remaining = 0

if count < limit then
  redis.call('ZADD', key, now, member)
  redis.call('PEXPIRE', key, window)
  allowed = 1
  remaining = limit - (count + 1)
else
  remaining = 0
end

local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
local reset = now + window
if oldest ~= nil and #oldest >= 2 then
  reset = tonumber(oldest[2]) + window
end

return {allowed, remaining, reset}
`)

// RedisStorage is a Redis-backed implementation of Storage.
type RedisStorage struct {
	client redis.UniversalClient
	script *redis.Script

	memberSeq atomic.Uint64

	closeOnce sync.Once
	closeErr  error
}

// NewRedisStorage constructs a Redis backend.
func NewRedisStorage(cfg *RedisConfig) (*RedisStorage, error) {
	conf, err := normalizeRedisConfig(cfg)
	if err != nil {
		return nil, err
	}

	client, err := newRedisClient(conf)
	if err != nil {
		return nil, err
	}

	s := &RedisStorage{
		client: client,
		script: redisSlidingWindowScript,
	}

	if err := s.pingWithRetry(context.Background(), conf.MaxRetries); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return s, nil
}

// CheckLimit atomically checks and updates the sliding-window state for key.
func (s *RedisStorage) CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	if key == "" {
		return false, 0, time.Time{}, fmt.Errorf("key is required")
	}
	if limit <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("limit must be positive, got %d", limit)
	}
	if window <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("window must be positive, got %s", window)
	}

	windowMS := window.Milliseconds()
	if windowMS <= 0 {
		return false, 0, time.Time{}, fmt.Errorf("window must be at least 1ms, got %s", window)
	}

	now := time.Now()
	nowMS := now.UnixMilli()
	member := fmt.Sprintf("%d-%d", now.UnixNano(), s.memberSeq.Add(1))
	redisKey := redisRateLimitPrefix + key

	res, err := s.script.Run(ctx, s.client, []string{redisKey}, nowMS, windowMS, limit, member).Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("running redis limit script: %w", err)
	}

	values, ok := res.([]interface{})
	if !ok || len(values) != 3 {
		return false, 0, time.Time{}, fmt.Errorf("unexpected redis script result: %T", res)
	}

	allowedRaw, err := asInt64(values[0])
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("parsing allowed result: %w", err)
	}
	remainingRaw, err := asInt64(values[1])
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("parsing remaining result: %w", err)
	}
	resetRaw, err := asInt64(values[2])
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("parsing reset result: %w", err)
	}

	return allowedRaw == 1, int(remainingRaw), time.UnixMilli(resetRaw), nil
}

// Close releases Redis resources. It is idempotent.
func (s *RedisStorage) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

func (s *RedisStorage) pingWithRetry(ctx context.Context, maxRetries int) error {
	attempts := maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	backoff := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := s.client.Ping(ctx).Err(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if i == attempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
	}

	if lastErr == nil {
		lastErr = errors.New("ping failed with unknown error")
	}
	return lastErr
}

func normalizeRedisConfig(cfg *RedisConfig) (*RedisConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("redis config is required")
	}

	conf := *cfg
	if conf.PoolSize <= 0 {
		conf.PoolSize = defaultRedisPoolSize
	}
	if conf.MaxRetries <= 0 {
		conf.MaxRetries = defaultRedisMaxRetries
	}
	if conf.DialTimeout <= 0 {
		conf.DialTimeout = defaultRedisDialTimeout
	}

	if conf.Cluster {
		if len(conf.ClusterNodes) == 0 {
			return nil, fmt.Errorf("cluster_nodes is required when cluster=true")
		}
	} else {
		if conf.Host == "" {
			return nil, fmt.Errorf("host is required when cluster=false")
		}
		if conf.Port <= 0 {
			return nil, fmt.Errorf("port must be positive when cluster=false, got %d", conf.Port)
		}
	}

	return &conf, nil
}

func newRedisClient(cfg *RedisConfig) (redis.UniversalClient, error) {
	if cfg.Cluster {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:       cfg.ClusterNodes,
			Password:    cfg.Password,
			PoolSize:    cfg.PoolSize,
			MaxRetries:  cfg.MaxRetries,
			DialTimeout: cfg.DialTimeout,
		}), nil
	}

	addr := cfg.Host + ":" + strconv.Itoa(cfg.Port)
	return redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    cfg.Password,
		DB:          cfg.DB,
		PoolSize:    cfg.PoolSize,
		MaxRetries:  cfg.MaxRetries,
		DialTimeout: cfg.DialTimeout,
	}), nil
}

func asInt64(v interface{}) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case float64:
		return int64(x), nil
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse int64 from %q: %w", x, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", v)
	}
}
