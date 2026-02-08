package recorder

import (
	"encoding/json"
	"io"
	"os"
	"sync"
)

// Recorder captures traffic records for later replay.
// Thread-safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	records []TrafficRecord
	writer  io.Writer // optional: stream records as they arrive
}

// New creates a new Recorder. If w is non-nil, records are also
// written to w as newline-delimited JSON as they arrive.
func New(w io.Writer) *Recorder {
	return &Recorder{
		writer: w,
	}
}

// Record captures a single traffic record.
func (r *Recorder) Record(rec TrafficRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = append(r.records, rec)

	if r.writer != nil {
		if err := json.NewEncoder(r.writer).Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

// Records returns a copy of all recorded traffic.
func (r *Recorder) Records() []TrafficRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]TrafficRecord, len(r.records))
	copy(out, r.records)
	return out
}

// Len returns the number of recorded items.
func (r *Recorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.records)
}

// ExportJSON writes all records to the given writer as a JSON array.
func (r *Recorder) ExportJSON(w io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.records)
}

// ExportFile writes all records to a file as a JSON array.
func (r *Recorder) ExportFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return r.ExportJSON(f)
}

// LoadJSON reads traffic records from a JSON array.
func LoadJSON(r io.Reader) ([]TrafficRecord, error) {
	var records []TrafficRecord
	if err := json.NewDecoder(r).Decode(&records); err != nil {
		return nil, err
	}
	return records, nil
}
