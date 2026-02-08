package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

// Server is the Chrono HTTP server that applies rate limiting to requests.
type Server struct {
	httpServer *http.Server
	limiter    limiter.Limiter
	clock      clock.Clock
	mux        *http.ServeMux
}

// New creates a new Chrono server.
func New(addr string, lim limiter.Limiter, clk clock.Clock) *Server {
	s := &Server{
		limiter: lim,
		clock:   clk,
		mux:     http.NewServeMux(),
	}
	s.routes()
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/check", s.handleCheck)
	s.mux.HandleFunc("/api/check/", s.handleCheckKey)
}

// handleRoot serves a welcome message.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"service": "chrono",
		"status":  "running",
		"time":    s.clock.Now().Format(time.RFC3339),
	})
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleCheck performs a rate limit check using the client IP as the key.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	key := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		key = forwarded
	}
	s.respondWithDecision(w, r, key)
}

// handleCheckKey performs a rate limit check using the key from the URL path.
// Path: /api/check/{key}
func (s *Server) handleCheckKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/api/check/"):]
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}
	s.respondWithDecision(w, r, key)
}

func (s *Server) respondWithDecision(w http.ResponseWriter, r *http.Request, key string) {
	decision := s.limiter.Allow(r.Context(), key)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", decision.Limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", decision.Remaining))
	w.Header().Set("X-RateLimit-Reset", decision.ResetAt.Format(time.RFC3339))

	if !decision.Allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(decision.RetryAt.Sub(s.clock.Now()).Seconds())+1))
		w.WriteHeader(http.StatusTooManyRequests)
	}

	json.NewEncoder(w).Encode(decision)
}

// Start begins listening. It blocks until the server is shut down.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	log.Printf("chrono server listening on %s", ln.Addr().String())
	return s.httpServer.Serve(ln)
}

// StartOnListener begins serving on the provided listener.
// Useful for tests that need to pick an ephemeral port.
func (s *Server) StartOnListener(ln net.Listener) error {
	log.Printf("chrono server listening on %s", ln.Addr().String())
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
