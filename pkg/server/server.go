package server

import (
	"net/http"

	internalserver "github.com/SmitUplenchwar2687/Chrono/internal/server"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	"github.com/SmitUplenchwar2687/Chrono/pkg/limiter"
	"github.com/SmitUplenchwar2687/Chrono/pkg/recorder"
)

// Server is the Chrono HTTP server that applies rate limiting to requests.
type Server = internalserver.Server

// Options configures optional server features.
type Options = internalserver.Options

// Hub manages WebSocket clients and broadcasts decision events.
type Hub = internalserver.Hub

// DashboardHTML is the embedded single-page dashboard.
const DashboardHTML = internalserver.DashboardHTML

// New creates a new Chrono server.
func New(addr string, lim limiter.Limiter, clk clock.Clock, opts ...Options) *Server {
	return internalserver.New(addr, lim, clk, opts...)
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return internalserver.NewHub()
}

// RecordingMiddleware wraps an http.Handler and records traffic.
func RecordingMiddleware(next http.Handler, rec *recorder.Recorder, clk clock.Clock) http.Handler {
	return internalserver.RecordingMiddleware(next, rec, clk)
}

// NewRecordingHandler creates a handler that records traffic.
func NewRecordingHandler(handler http.Handler, rec *recorder.Recorder, clk clock.Clock) http.Handler {
	return internalserver.NewRecordingHandler(handler, rec, clk)
}
