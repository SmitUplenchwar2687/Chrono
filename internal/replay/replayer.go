package replay

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

// Replayer replays recorded traffic through a rate limiter at a configurable speed.
type Replayer struct {
	records []recorder.TrafficRecord
	limiter limiter.Limiter
	clock   *clock.VirtualClock
	filter  Filter
	speed   float64 // 1.0 = real-time, 10.0 = 10x, 0 = instant
}

// Result captures the outcome of replaying a single record.
type Result struct {
	Record   recorder.TrafficRecord `json:"record"`
	Decision limiter.Decision       `json:"decision"`
	Time     time.Time              `json:"time"` // virtual time when decision was made
}

// Summary aggregates replay statistics.
type Summary struct {
	TotalRecords   int            `json:"total_records"`
	Filtered       int            `json:"filtered"`
	Replayed       int            `json:"replayed"`
	Allowed        int            `json:"allowed"`
	Denied         int            `json:"denied"`
	Duration       time.Duration  `json:"duration"`         // virtual time span
	WallDuration   time.Duration  `json:"wall_duration"`    // actual wall clock time
	PerKey         map[string]KeySummary `json:"per_key"`
}

// KeySummary has per-key stats.
type KeySummary struct {
	Allowed int `json:"allowed"`
	Denied  int `json:"denied"`
}

// New creates a new replayer.
func New(lim limiter.Limiter, vc *clock.VirtualClock, speed float64, filter Filter) *Replayer {
	if speed < 0 {
		speed = 0
	}
	return &Replayer{
		limiter: lim,
		clock:   vc,
		speed:   speed,
		filter:  filter,
	}
}

// Load reads traffic records from a JSON reader.
func (r *Replayer) Load(reader io.Reader) error {
	records, err := recorder.LoadJSON(reader)
	if err != nil {
		return fmt.Errorf("loading records: %w", err)
	}
	r.records = records
	return nil
}

// LoadRecords sets the records directly.
func (r *Replayer) LoadRecords(records []recorder.TrafficRecord) {
	r.records = make([]recorder.TrafficRecord, len(records))
	copy(r.records, records)
}

// Run replays all loaded records through the limiter.
// The callback is called for each replayed record with its decision.
// Returns a summary of the replay.
func (r *Replayer) Run(ctx context.Context, cb func(Result)) (*Summary, error) {
	if len(r.records) == 0 {
		return nil, fmt.Errorf("no records loaded")
	}

	// Sort records by timestamp.
	sorted := make([]recorder.TrafficRecord, len(r.records))
	copy(sorted, r.records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Apply filter.
	var filtered []recorder.TrafficRecord
	for _, rec := range sorted {
		if r.filter.Match(rec) {
			filtered = append(filtered, rec)
		}
	}

	if len(filtered) == 0 {
		return &Summary{
			TotalRecords: len(sorted),
			Filtered:     0,
			PerKey:       make(map[string]KeySummary),
		}, nil
	}

	summary := &Summary{
		TotalRecords: len(sorted),
		Filtered:     len(filtered),
		PerKey:       make(map[string]KeySummary),
	}

	wallStart := time.Now()
	baseTime := filtered[0].Timestamp

	for i, rec := range filtered {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}

		// Advance virtual clock to match the record's timestamp offset.
		if i > 0 {
			gap := rec.Timestamp.Sub(filtered[i-1].Timestamp)
			if gap > 0 {
				if r.speed > 0 {
					// Sleep for scaled wall-clock time for visual effect.
					scaledGap := time.Duration(float64(gap) / r.speed)
					if scaledGap > time.Millisecond {
						select {
						case <-ctx.Done():
							return summary, ctx.Err()
						case <-time.After(scaledGap):
						}
					}
				}
				r.clock.Advance(gap)
			}
		}

		decision := r.limiter.Allow(ctx, rec.Key)
		result := Result{
			Record:   rec,
			Decision: decision,
			Time:     r.clock.Now(),
		}

		summary.Replayed++
		if decision.Allowed {
			summary.Allowed++
		} else {
			summary.Denied++
		}
		ks := summary.PerKey[rec.Key]
		if decision.Allowed {
			ks.Allowed++
		} else {
			ks.Denied++
		}
		summary.PerKey[rec.Key] = ks

		if cb != nil {
			cb(result)
		}
	}

	lastTime := filtered[len(filtered)-1].Timestamp
	summary.Duration = lastTime.Sub(baseTime)
	summary.WallDuration = time.Since(wallStart)

	return summary, nil
}
