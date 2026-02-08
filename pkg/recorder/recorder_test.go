package recorder

import (
	"testing"
	"time"
)

func TestRecorderRoundtrip(t *testing.T) {
	rec := New(nil)
	err := rec.Record(TrafficRecord{
		Timestamp: time.Now(),
		Key:       "user1",
		Endpoint:  "GET /api/profile",
	})
	if err != nil {
		t.Fatalf("Record() failed: %v", err)
	}
	if rec.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", rec.Len())
	}
}
