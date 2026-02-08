package replay

import (
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

func TestFilter_Empty_MatchesAll(t *testing.T) {
	f := Filter{}
	r := recorder.TrafficRecord{Timestamp: epoch, Key: "any", Endpoint: "GET /any"}
	if !f.Match(r) {
		t.Error("empty filter should match all records")
	}
}

func TestFilter_Keys(t *testing.T) {
	f := Filter{Keys: []string{"user1", "user2"}}

	if !f.Match(recorder.TrafficRecord{Key: "user1"}) {
		t.Error("should match user1")
	}
	if !f.Match(recorder.TrafficRecord{Key: "user2"}) {
		t.Error("should match user2")
	}
	if f.Match(recorder.TrafficRecord{Key: "user3"}) {
		t.Error("should not match user3")
	}
}

func TestFilter_Endpoints(t *testing.T) {
	f := Filter{Endpoints: []string{"/api"}}

	if !f.Match(recorder.TrafficRecord{Endpoint: "GET /api/data"}) {
		t.Error("should match GET /api/data")
	}
	if !f.Match(recorder.TrafficRecord{Endpoint: "POST /api/users"}) {
		t.Error("should match POST /api/users")
	}
	if f.Match(recorder.TrafficRecord{Endpoint: "GET /health"}) {
		t.Error("should not match GET /health")
	}
}

func TestFilter_After(t *testing.T) {
	f := Filter{After: epoch.Add(5 * time.Minute)}

	if f.Match(recorder.TrafficRecord{Timestamp: epoch}) {
		t.Error("should not match record before After")
	}
	if f.Match(recorder.TrafficRecord{Timestamp: epoch.Add(5 * time.Minute)}) {
		t.Error("should not match record at exact After boundary")
	}
	if !f.Match(recorder.TrafficRecord{Timestamp: epoch.Add(6 * time.Minute)}) {
		t.Error("should match record after After")
	}
}

func TestFilter_Before(t *testing.T) {
	f := Filter{Before: epoch.Add(5 * time.Minute)}

	if !f.Match(recorder.TrafficRecord{Timestamp: epoch}) {
		t.Error("should match record before Before")
	}
	if f.Match(recorder.TrafficRecord{Timestamp: epoch.Add(5 * time.Minute)}) {
		t.Error("should not match record at exact Before boundary")
	}
	if f.Match(recorder.TrafficRecord{Timestamp: epoch.Add(6 * time.Minute)}) {
		t.Error("should not match record after Before")
	}
}

func TestFilter_Combined(t *testing.T) {
	f := Filter{
		Keys:      []string{"user1"},
		Endpoints: []string{"/api"},
		After:     epoch,
		Before:    epoch.Add(10 * time.Minute),
	}

	// Matches all criteria.
	if !f.Match(recorder.TrafficRecord{
		Timestamp: epoch.Add(5 * time.Minute),
		Key:       "user1",
		Endpoint:  "GET /api/data",
	}) {
		t.Error("should match record meeting all criteria")
	}

	// Wrong key.
	if f.Match(recorder.TrafficRecord{
		Timestamp: epoch.Add(5 * time.Minute),
		Key:       "user2",
		Endpoint:  "GET /api/data",
	}) {
		t.Error("should not match wrong key")
	}

	// Wrong time.
	if f.Match(recorder.TrafficRecord{
		Timestamp: epoch.Add(15 * time.Minute),
		Key:       "user1",
		Endpoint:  "GET /api/data",
	}) {
		t.Error("should not match record outside time range")
	}
}
