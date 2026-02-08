package recorder

import (
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

// TrafficRecord represents a single captured request.
type TrafficRecord struct {
	Timestamp time.Time         `json:"timestamp"`
	Key       string            `json:"key"`                 // User ID, API key, IP, etc.
	Endpoint  string            `json:"endpoint"`            // e.g., "GET /api/users"
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// DecisionEvent pairs a traffic record with the rate limit decision it produced.
// Used for streaming to the dashboard and replay output.
type DecisionEvent struct {
	Record   TrafficRecord    `json:"record"`
	Decision limiter.Decision `json:"decision"`
	Time     time.Time        `json:"time"`
}
