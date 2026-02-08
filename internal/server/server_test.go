package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func startTestServer(t *testing.T, lim limiter.Limiter, clk clock.Clock, opts ...Options) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(ln.Addr().String(), lim, clk, opts...)
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

// === Dashboard tests ===

func TestServer_Dashboard_Redirect(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(baseURL + "/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", resp.StatusCode)
	}
}

func TestServer_Dashboard_Serves_HTML(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	baseURL, cleanup := startTestServer(t, lim, vc)
	defer cleanup()

	resp, err := http.Get(baseURL + "/dashboard/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// === WebSocket tests ===

func TestServer_WebSocket_Connect(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	hub := NewHub()
	baseURL, cleanup := startTestServer(t, lim, vc, Options{Hub: hub})
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Give a moment for the connection to register.
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("client count = %d, want 1", hub.ClientCount())
	}
}

func TestServer_WebSocket_ReceivesDecisions(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	hub := NewHub()
	baseURL, cleanup := startTestServer(t, lim, vc, Options{Hub: hub})
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Make a rate limit check — should broadcast to WS.
	resp, err := http.Get(baseURL + "/api/check/user1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Read the WebSocket message.
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	var event recorder.DecisionEvent
	json.Unmarshal(msg, &event)
	if event.Record.Key != "user1" {
		t.Errorf("event key = %q, want %q", event.Record.Key, "user1")
	}
	if !event.Decision.Allowed {
		t.Error("event decision should be allowed")
	}
}

// === Recording tests ===

func TestServer_Recording(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	rec := recorder.New(nil)
	baseURL, cleanup := startTestServer(t, lim, vc, Options{Recorder: rec})
	defer cleanup()

	// Make some requests.
	for i := 0; i < 3; i++ {
		resp, err := http.Get(baseURL + "/api/check/user1")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if rec.Len() != 3 {
		t.Errorf("recorded %d requests, want 3", rec.Len())
	}

	records := rec.Records()
	if records[0].Key != "user1" {
		t.Errorf("record[0].Key = %q, want %q", records[0].Key, "user1")
	}
}
