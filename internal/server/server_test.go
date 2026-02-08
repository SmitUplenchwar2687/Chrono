package server

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func startTestServer(t *testing.T, lim limiter.Limiter, clk clock.Clock) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ln.Addr().String(), lim, clk)
	go srv.StartOnListener(ln)
	baseURL := "http://" + ln.Addr().String()
	return baseURL, func() {
		srv.Shutdown(nil)
	}
}

func TestServer_Root(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["service"] != "chrono" {
		t.Errorf("service = %q, want %q", body["service"], "chrono")
	}
}

func TestServer_Health(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestServer_NotFound(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_CheckKey_Allowed(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/api/check/user1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var decision limiter.Decision
	json.NewDecoder(resp.Body).Decode(&decision)
	if !decision.Allowed {
		t.Error("first request should be allowed")
	}
	if decision.Remaining != 9 {
		t.Errorf("remaining = %d, want 9", decision.Remaining)
	}

	// Check rate limit headers.
	if resp.Header.Get("X-RateLimit-Limit") != "10" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", resp.Header.Get("X-RateLimit-Limit"), "10")
	}
}

func TestServer_CheckKey_Denied(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(3, time.Minute, 3, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	// Exhaust the limit.
	for i := 0; i < 3; i++ {
		resp, err := http.Get(baseURL + "/api/check/user1")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// 4th request should be denied.
	resp, err := http.Get(baseURL + "/api/check/user1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}

	var decision limiter.Decision
	json.NewDecoder(resp.Body).Decode(&decision)
	if decision.Allowed {
		t.Error("4th request should be denied")
	}

	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After header should be set")
	}
}

func TestServer_CheckKey_EmptyKey(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/api/check/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServer_Check_UsesIP(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/api/check")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var decision limiter.Decision
	json.NewDecoder(resp.Body).Decode(&decision)
	if !decision.Allowed {
		t.Error("first IP-based check should be allowed")
	}
}

func TestServer_SeparateKeysAreSeparate(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(1, time.Minute, 1, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	// Exhaust user1.
	resp, err := http.Get(baseURL + "/api/check/user1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(baseURL + "/api/check/user1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("user1 2nd request: status = %d, want 429", resp.StatusCode)
	}

	// user2 should still be allowed.
	resp, err = http.Get(baseURL + "/api/check/user2")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("user2 1st request: status = %d, want 200", resp.StatusCode)
	}
}
