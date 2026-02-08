package server

import (
	"log"
	"net/http"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

// RecordingMiddleware wraps an http.Handler and records traffic.
func RecordingMiddleware(next http.Handler, rec *recorder.Recorder, clk clock.Clock) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			key = forwarded
		}

		tr := recorder.TrafficRecord{
			Timestamp: clk.Now(),
			Key:       key,
			Endpoint:  r.Method + " " + r.URL.Path,
			Metadata: map[string]string{
				"user_agent": r.UserAgent(),
			},
		}

		if err := rec.Record(tr); err != nil {
			log.Printf("record error: %v", err)
		}
		next.ServeHTTP(w, r)
	})
}

// NewRecordingHandler creates a handler that records traffic and applies rate limiting.
// It records every request and optionally broadcasts decisions to the WebSocket hub.
func NewRecordingHandler(
	handler http.Handler,
	rec *recorder.Recorder,
	clk clock.Clock,
) http.Handler {
	_ = time.Now // ensure time is imported (used by recorder)
	return RecordingMiddleware(handler, rec, clk)
}
