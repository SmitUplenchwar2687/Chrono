package recorder

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestRecorder_Record(t *testing.T) {
	rec := New(nil)

	err := rec.Record(TrafficRecord{
		Timestamp: epoch,
		Key:       "user1",
		Endpoint:  "GET /api/data",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Len() != 1 {
		t.Errorf("Len() = %d, want 1", rec.Len())
	}
}

func TestRecorder_Records_ReturnsCopy(t *testing.T) {
	rec := New(nil)
	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user1", Endpoint: "GET /"})

	records := rec.Records()
	records[0].Key = "mutated"

	original := rec.Records()
	if original[0].Key != "user1" {
		t.Error("Records() should return a copy, original was mutated")
	}
}

func TestRecorder_StreamToWriter(t *testing.T) {
	var buf bytes.Buffer
	rec := New(&buf)

	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user1", Endpoint: "GET /api"})
	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user2", Endpoint: "POST /api"})

	// Should have 2 newline-delimited JSON lines.
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var r TrafficRecord
	json.Unmarshal(lines[0], &r)
	if r.Key != "user1" {
		t.Errorf("first record key = %q, want %q", r.Key, "user1")
	}
}

func TestRecorder_ExportJSON(t *testing.T) {
	rec := New(nil)
	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user1", Endpoint: "GET /a"})
	rec.Record(TrafficRecord{Timestamp: epoch.Add(time.Second), Key: "user2", Endpoint: "GET /b"})

	var buf bytes.Buffer
	err := rec.ExportJSON(&buf)
	if err != nil {
		t.Fatal(err)
	}

	var records []TrafficRecord
	json.NewDecoder(&buf).Decode(&records)
	if len(records) != 2 {
		t.Fatalf("exported %d records, want 2", len(records))
	}
	if records[0].Key != "user1" {
		t.Errorf("records[0].Key = %q, want %q", records[0].Key, "user1")
	}
	if records[1].Key != "user2" {
		t.Errorf("records[1].Key = %q, want %q", records[1].Key, "user2")
	}
}

func TestRecorder_ExportFile(t *testing.T) {
	rec := New(nil)
	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user1", Endpoint: "GET /a"})

	path := filepath.Join(t.TempDir(), "traffic.json")
	err := rec.ExportFile(path)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var records []TrafficRecord
	json.Unmarshal(data, &records)
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}
}

func TestLoadJSON(t *testing.T) {
	input := `[
		{"timestamp": "2024-01-01T00:00:00Z", "key": "user1", "endpoint": "GET /a"},
		{"timestamp": "2024-01-01T00:00:01Z", "key": "user2", "endpoint": "POST /b"}
	]`

	records, err := LoadJSON(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("loaded %d records, want 2", len(records))
	}
	if records[0].Key != "user1" {
		t.Errorf("records[0].Key = %q, want %q", records[0].Key, "user1")
	}
}

func TestLoadJSON_Roundtrip(t *testing.T) {
	rec := New(nil)
	rec.Record(TrafficRecord{Timestamp: epoch, Key: "user1", Endpoint: "GET /a", Metadata: map[string]string{"region": "us"}})
	rec.Record(TrafficRecord{Timestamp: epoch.Add(5 * time.Second), Key: "user2", Endpoint: "POST /b"})

	var buf bytes.Buffer
	rec.ExportJSON(&buf)

	loaded, err := LoadJSON(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("roundtrip: got %d records, want 2", len(loaded))
	}
	if loaded[0].Metadata["region"] != "us" {
		t.Error("metadata not preserved in roundtrip")
	}
}

func TestRecorder_ConcurrentAccess(t *testing.T) {
	rec := New(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.Record(TrafficRecord{Timestamp: epoch, Key: "user", Endpoint: "GET /"})
		}()
	}
	wg.Wait()

	if rec.Len() != 100 {
		t.Errorf("Len() = %d, want 100", rec.Len())
	}
}
