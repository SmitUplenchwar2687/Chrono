package replay

import (
	"strings"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

// Filter defines criteria for selecting traffic records during replay.
type Filter struct {
	Keys      []string  // Only include these keys (empty = all)
	Endpoints []string  // Only include these endpoints (empty = all)
	After     time.Time // Only include records after this time (zero = no limit)
	Before    time.Time // Only include records before this time (zero = no limit)
}

// Match returns true if the record passes the filter.
func (f *Filter) Match(r recorder.TrafficRecord) bool {
	if len(f.Keys) > 0 && !contains(f.Keys, r.Key) {
		return false
	}
	if len(f.Endpoints) > 0 && !matchEndpoint(f.Endpoints, r.Endpoint) {
		return false
	}
	if !f.After.IsZero() && !r.Timestamp.After(f.After) {
		return false
	}
	if !f.Before.IsZero() && !r.Timestamp.Before(f.Before) {
		return false
	}
	return true
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func matchEndpoint(patterns []string, endpoint string) bool {
	for _, p := range patterns {
		if p == endpoint || strings.Contains(endpoint, p) {
			return true
		}
	}
	return false
}
