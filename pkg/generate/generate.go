package generate

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/recorder"
)

const (
	// PatternSteady generates evenly distributed traffic.
	PatternSteady = "steady"
	// PatternBurst generates clustered bursts with quiet gaps.
	PatternBurst = "burst"
	// PatternRamp generates traffic density that increases over time.
	PatternRamp = "ramp"
)

// DefaultEndpoints is the default endpoint pool used by GenerateTraffic
// when Options.Endpoints is not provided.
var DefaultEndpoints = []string{
	"GET /api/users",
	"GET /api/data",
	"POST /api/events",
	"GET /api/search",
	"PUT /api/settings",
}

// Options controls how synthetic traffic is generated.
type Options struct {
	Count     int
	Keys      int
	Duration  time.Duration
	Pattern   string
	Start     time.Time
	Seed      int64
	Endpoints []string
}

// DefaultOptions returns defaults aligned with Chrono CLI behavior.
func DefaultOptions() Options {
	return Options{
		Count:    100,
		Keys:     3,
		Duration: 5 * time.Minute,
		Pattern:  PatternSteady,
	}
}

// GenerateTraffic creates synthetic traffic records based on the provided options.
func GenerateTraffic(opts Options) ([]recorder.TrafficRecord, error) {
	if opts.Count <= 0 {
		return nil, fmt.Errorf("count must be positive, got %d", opts.Count)
	}
	if opts.Keys <= 0 {
		return nil, fmt.Errorf("keys must be positive, got %d", opts.Keys)
	}
	if opts.Duration <= 0 {
		return nil, fmt.Errorf("duration must be positive, got %s", opts.Duration)
	}

	if opts.Pattern == "" {
		opts.Pattern = PatternSteady
	}
	if opts.Start.IsZero() {
		opts.Start = time.Now().Truncate(time.Second)
	}
	if len(opts.Endpoints) == 0 {
		opts.Endpoints = DefaultEndpoints
	}
	if opts.Seed == 0 {
		opts.Seed = time.Now().UnixNano()
	}

	rng := rand.New(rand.NewSource(opts.Seed))
	userKeys := makeUserKeys(opts.Keys)

	switch opts.Pattern {
	case PatternBurst:
		return generateBurst(rng, opts.Start, opts.Count, userKeys, opts.Endpoints, opts.Duration), nil
	case PatternRamp:
		return generateRamp(rng, opts.Start, opts.Count, userKeys, opts.Endpoints, opts.Duration), nil
	default: // steady and unknown patterns default to steady behavior.
		return generateSteady(rng, opts.Start, opts.Count, userKeys, opts.Endpoints, opts.Duration), nil
	}
}

func makeUserKeys(numKeys int) []string {
	userKeys := make([]string, numKeys)
	for i := range userKeys {
		userKeys[i] = fmt.Sprintf("user-%d", i+1)
	}
	return userKeys
}

func generateSteady(rng *rand.Rand, start time.Time, count int, keys, endpoints []string, dur time.Duration) []recorder.TrafficRecord {
	interval := dur / time.Duration(count)
	records := make([]recorder.TrafficRecord, count)
	for i := range records {
		records[i] = recorder.TrafficRecord{
			Timestamp: start.Add(time.Duration(i) * interval),
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		}
	}
	return records
}

func generateBurst(rng *rand.Rand, start time.Time, count int, keys, endpoints []string, dur time.Duration) []recorder.TrafficRecord {
	records := make([]recorder.TrafficRecord, 0, count)
	numBursts := 4
	burstSize := count / numBursts
	burstGap := dur / time.Duration(numBursts)

	for b := 0; b < numBursts; b++ {
		burstStart := start.Add(time.Duration(b) * burstGap)
		for i := 0; i < burstSize; i++ {
			offset := time.Duration(rng.Intn(1000)) * time.Millisecond
			records = append(records, recorder.TrafficRecord{
				Timestamp: burstStart.Add(offset),
				Key:       keys[rng.Intn(len(keys))],
				Endpoint:  endpoints[rng.Intn(len(endpoints))],
			})
		}
	}

	for len(records) < count {
		records = append(records, recorder.TrafficRecord{
			Timestamp: start.Add(time.Duration(rng.Int63n(int64(dur)))),
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		})
	}

	return records
}

func generateRamp(rng *rand.Rand, start time.Time, count int, keys, endpoints []string, dur time.Duration) []recorder.TrafficRecord {
	records := make([]recorder.TrafficRecord, 0, count)
	for i := 0; i < count; i++ {
		frac := float64(i) / float64(count)
		t := start.Add(time.Duration(frac * frac * float64(dur)))
		records = append(records, recorder.TrafficRecord{
			Timestamp: t,
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		})
	}
	return records
}
