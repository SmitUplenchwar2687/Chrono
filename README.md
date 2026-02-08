# Chrono

**Time-travel testing for rate limits.**

Chrono is a developer tool that lets you test rate limiting without waiting for real time to pass. Fast-forward hours in milliseconds, replay production traffic at any speed, and watch rate limit decisions on a live dashboard.

## Why Chrono?

Testing rate limiters is painful. You set a limit of "100 requests per hour" and then... wait an hour? Run flaky tests with `time.Sleep`? Chrono solves this with a **virtual clock** that you control:

- **Time Travel** - Fast-forward 1 hour in 5 milliseconds. Tokens refill instantly.
- **Traffic Replay** - Record production traffic, replay it locally at 100x speed.
- **Visual Debugging** - Live dashboard showing exactly why requests were allowed or denied.
- **Multiple Algorithms** - Token bucket, sliding window, and fixed window.

## Installation

```bash
go install github.com/SmitUplenchwar2687/Chrono/cmd/chrono@latest
```

Or build from source:

```bash
git clone https://github.com/SmitUplenchwar2687/Chrono.git
cd Chrono
make build
./bin/chrono --help
```

## Use As A Library

Chrono now exposes public Go packages for embedding core features in other projects:

- `github.com/SmitUplenchwar2687/Chrono/pkg/cli`
- `github.com/SmitUplenchwar2687/Chrono/pkg/clock`
- `github.com/SmitUplenchwar2687/Chrono/pkg/config`
- `github.com/SmitUplenchwar2687/Chrono/pkg/generate`
- `github.com/SmitUplenchwar2687/Chrono/pkg/limiter`
- `github.com/SmitUplenchwar2687/Chrono/pkg/recorder`
- `github.com/SmitUplenchwar2687/Chrono/pkg/replay`
- `github.com/SmitUplenchwar2687/Chrono/pkg/server`
- `github.com/SmitUplenchwar2687/Chrono/pkg/storage`

```go
package main

import (
	"context"
	"fmt"
	"time"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	chronolimiter "github.com/SmitUplenchwar2687/Chrono/pkg/limiter"
)

func main() {
	clk := chronoclock.NewRealClock()
	lim := chronolimiter.NewTokenBucket(10, time.Minute, 10, clk)

	d := lim.Allow(context.Background(), "user-123")
	fmt.Printf("allowed=%v remaining=%d\n", d.Allowed, d.Remaining)
}
```

## Quick Start

### 1. Test rate limiting with time travel

```bash
chrono test --rate 5 --window 1m --requests 8 --fast-forward 1m
```

```
=== Chrono Rate Limit Test ===

--- Initial requests ---
  #001 [ALLOW] key=test-user remaining=4/5
  #002 [ALLOW] key=test-user remaining=3/5
  #003 [ALLOW] key=test-user remaining=2/5
  #004 [ALLOW] key=test-user remaining=1/5
  #005 [ALLOW] key=test-user remaining=0/5
  #006 [DENY ] key=test-user remaining=0/5
  #007 [DENY ] key=test-user remaining=0/5
  #008 [DENY ] key=test-user remaining=0/5

--- After fast-forward 1m0s ---
  #001 [ALLOW] key=test-user remaining=4/5
  ...

Time travel worked! Requests were denied, then
allowed again after fast-forwarding the clock.
```

### 2. Start the server with a live dashboard

```bash
chrono dashboard --rate 10 --window 30s
```

Open http://localhost:8080/dashboard/ in your browser, then send requests:

```bash
# In another terminal
for i in $(seq 1 20); do curl -s localhost:8080/api/check/user1 > /dev/null; done
```

Watch ALLOW/DENY decisions appear in real time with remaining token counts.

### 3. Record and replay traffic

```bash
# Start server with recording enabled
chrono server --rate 10 --window 1m --record traffic.json

# Send some traffic, then Ctrl+C to stop and export

# Replay the recorded traffic through a different algorithm
chrono replay --file traffic.json --algorithm sliding_window --rate 5 --window 30s
```

```
--- Replay Summary ---
  Total records:  10
  Filtered:       10
  Replayed:       10
  Allowed:        8
  Denied:         2
  Deny rate: 20.0% (2/10 requests denied)
```

## Commands

| Command | Description |
|---------|-------------|
| `chrono test` | Run rate limit tests with time travel |
| `chrono server` | Start HTTP server with rate limiting |
| `chrono dashboard` | Start server with live visual dashboard |
| `chrono replay` | Replay recorded traffic through a limiter |

### `chrono test`

```bash
chrono test --rate 10 --window 1m --requests 20 --fast-forward 1h
chrono test --algorithm sliding_window --keys user1,user2 --json
```

| Flag | Default | Description |
|------|---------|-------------|
| `--algorithm` | `token_bucket` | Rate limiting algorithm |
| `--rate` | `10` | Requests allowed per window |
| `--window` | `1m` | Window duration |
| `--burst` | `0` | Max burst (token bucket only) |
| `--requests` | `15` | Requests per batch |
| `--keys` | `test-user` | Comma-separated keys to test |
| `--fast-forward` | `0` | Time to skip between batches |
| `--json` | `false` | Output as JSON |

### `chrono server`

```bash
chrono server --addr :9090 --algorithm sliding_window --rate 100 --window 1m
chrono server --record traffic.json
```

**Endpoints:**

| Endpoint | Description |
|----------|-------------|
| `GET /` | Server info and current time |
| `GET /health` | Health check |
| `GET /api/check` | Rate limit check (uses client IP) |
| `GET /api/check/{key}` | Rate limit check for a specific key |
| `GET /dashboard/` | Live visual dashboard |
| `WS /ws` | WebSocket for real-time events |

**Response headers:** `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After` (on 429).

### `chrono replay`

```bash
chrono replay --file traffic.json --speed 100 --algorithm token_bucket
chrono replay --file traffic.json --keys user1 --endpoints /api --json
```

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | (required) | Path to recorded traffic JSON |
| `--speed` | `0` | Replay speed (0=instant, 1=real-time, 10=10x) |
| `--keys` | (all) | Filter by keys |
| `--endpoints` | (all) | Filter by endpoint patterns |
| `--json` | `false` | Output as JSON |

## Algorithms

### Token Bucket

Tokens accumulate at a steady rate. Each request consumes one token. Supports burst capacity exceeding the steady-state rate.

```bash
chrono test --algorithm token_bucket --rate 10 --window 1m --burst 20
```

### Sliding Window

Tracks individual request timestamps. Counts how many fall within the current window. Precise, no boundary issues, but uses more memory.

```bash
chrono test --algorithm sliding_window --rate 10 --window 1m
```

### Fixed Window

Divides time into fixed intervals with a counter per window. Simple and memory-efficient, but can allow up to 2x the rate at window boundaries.

```bash
chrono test --algorithm fixed_window --rate 10 --window 1m
```

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                     CLI (Cobra)                      │
│  server  │  test  │  replay  │  dashboard            │
└────┬─────┴────┬───┴─────┬───┴──────┬─────────────────┘
     │          │         │          │
     ▼          ▼         ▼          ▼
┌──────────┐ ┌──────┐ ┌────────┐ ┌───────────┐
│  HTTP    │ │ Test │ │ Replay │ │ Dashboard │
│  Server  │ │Runner│ │ Engine │ │ WebSocket │
└────┬─────┘ └──┬───┘ └───┬────┘ └─────┬─────┘
     └──────────┼─────────┘            │
                ▼                      │
     ┌─────────────────────┐           │
     │   Rate Limiter      │◄──────────┘
     │  Token | Sliding |  │
     │  Bucket| Window  |  │
     │        | Fixed   |  │
     └────────┬────────────┘
              ▼
     ┌─────────────────────┐
     │    Virtual Clock    │ ◄── TIME TRAVEL
     └─────────────────────┘
```

The key design: every component uses a `Clock` interface. Swap in a `VirtualClock` and you control time.

## Project Structure

```
chrono/
├── cmd/chrono/main.go           # Entry point
├── pkg/
│   ├── cli/                     # Public CLI entrypoint
│   ├── clock/                   # Public clock API
│   ├── config/                  # Public config API
│   ├── generate/                # Public traffic generation API
│   ├── limiter/                 # Public limiter API
│   ├── recorder/                # Public recorder API
│   ├── replay/                  # Public replay API
│   ├── server/                  # Public HTTP server API
│   └── storage/                 # Public storage API
├── internal/
│   ├── cli/                     # Cobra CLI commands
│   ├── clock/                   # Clock interface + VirtualClock
│   ├── limiter/                 # Rate limiting algorithms
│   ├── storage/                 # Storage backends (in-memory)
│   ├── recorder/                # Traffic recording + JSON export
│   ├── replay/                  # Replay engine + filters
│   ├── server/                  # HTTP server + WebSocket + dashboard
│   └── config/                  # Configuration
├── Makefile
├── go.mod
└── go.sum
```

## Development

```bash
make test       # Run all tests with race detector
make build      # Build binary to bin/chrono
make coverage   # Run tests with coverage report
make bench      # Run benchmarks
make lint       # Run golangci-lint
make all        # Test + build
```

## Traffic File Format

Traffic is recorded and replayed as JSON:

```json
[
  {
    "timestamp": "2024-01-01T00:00:00Z",
    "key": "user1",
    "endpoint": "GET /api/data",
    "metadata": {"region": "us-east"}
  }
]
```

## License

MIT
