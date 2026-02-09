package generate

import (
	"testing"
	"time"
)

func TestGenerateTraffic_AllPatterns(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	patterns := []string{PatternSteady, PatternBurst, PatternRamp}

	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			records, err := GenerateTraffic(&Options{
				Count:    32,
				Keys:     3,
				Duration: 2 * time.Minute,
				Pattern:  p,
				Start:    start,
				Seed:     7,
			})
			if err != nil {
				t.Fatalf("GenerateTraffic() error = %v", err)
			}
			if len(records) != 32 {
				t.Fatalf("len(records) = %d, want 32", len(records))
			}
			for _, rec := range records {
				if rec.Key == "" || rec.Endpoint == "" {
					t.Fatalf("record should have key and endpoint: %+v", rec)
				}
			}
		})
	}
}

func TestGenerateTraffic_UnknownPatternFallsBackToSteady(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	records, err := GenerateTraffic(&Options{
		Count:    10,
		Keys:     2,
		Duration: 10 * time.Second,
		Pattern:  "not-a-pattern",
		Start:    start,
		Seed:     1,
	})
	if err != nil {
		t.Fatalf("GenerateTraffic() error = %v", err)
	}

	// Steady pattern uses a fixed interval.
	if !records[1].Timestamp.Equal(start.Add(time.Second)) {
		t.Fatalf("unexpected timestamp at index 1: got %v", records[1].Timestamp)
	}
}

func TestGenerateTraffic_InvalidOptions(t *testing.T) {
	_, err := GenerateTraffic(&Options{
		Count:    0,
		Keys:     1,
		Duration: time.Minute,
		Pattern:  PatternSteady,
	})
	if err == nil {
		t.Fatal("expected error for count=0")
	}

	_, err = GenerateTraffic(&Options{
		Count:    1,
		Keys:     0,
		Duration: time.Minute,
		Pattern:  PatternSteady,
	})
	if err == nil {
		t.Fatal("expected error for keys=0")
	}

	_, err = GenerateTraffic(&Options{
		Count:    1,
		Keys:     1,
		Duration: 0,
		Pattern:  PatternSteady,
	})
	if err == nil {
		t.Fatal("expected error for duration=0")
	}
}

func TestGenerateTraffic_NilOptions(t *testing.T) {
	_, err := GenerateTraffic(nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}
}
