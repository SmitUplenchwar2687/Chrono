package recorder

import (
	"io"

	internalrecorder "github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

// TrafficRecord represents a single captured request.
type TrafficRecord = internalrecorder.TrafficRecord

// DecisionEvent pairs a traffic record with the produced decision.
type DecisionEvent = internalrecorder.DecisionEvent

// Recorder captures traffic records for later replay.
type Recorder = internalrecorder.Recorder

// New creates a new Recorder.
func New(w io.Writer) *Recorder {
	return internalrecorder.New(w)
}

// LoadJSON reads traffic records from a JSON array.
func LoadJSON(r io.Reader) ([]TrafficRecord, error) {
	return internalrecorder.LoadJSON(r)
}
