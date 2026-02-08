package limiter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// BenchmarkTokenBucket_SingleKey measures throughput for a single key.
func BenchmarkTokenBucket_SingleKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(1000000, time.Minute, 1000000, vc)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow(ctx, "user1")
	}
}

// BenchmarkTokenBucket_MultiKey measures throughput across many keys.
func BenchmarkTokenBucket_MultiKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(1000, time.Minute, 1000, vc)
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow(ctx, keys[i%len(keys)])
	}
}

// BenchmarkTokenBucket_Parallel measures concurrent throughput.
func BenchmarkTokenBucket_Parallel(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(1000000, time.Minute, 1000000, vc)
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tb.Allow(ctx, fmt.Sprintf("user-%d", i%100))
			i++
		}
	})
}

// BenchmarkSlidingWindow_SingleKey measures throughput for a single key.
func BenchmarkSlidingWindow_SingleKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(1000000, time.Minute, vc)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow(ctx, "user1")
	}
}

// BenchmarkSlidingWindow_MultiKey measures throughput across many keys.
func BenchmarkSlidingWindow_MultiKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(1000, time.Minute, vc)
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow(ctx, keys[i%len(keys)])
	}
}

// BenchmarkSlidingWindow_Parallel measures concurrent throughput.
func BenchmarkSlidingWindow_Parallel(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(1000000, time.Minute, vc)
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sw.Allow(ctx, fmt.Sprintf("user-%d", i%100))
			i++
		}
	})
}

// BenchmarkFixedWindow_SingleKey measures throughput for a single key.
func BenchmarkFixedWindow_SingleKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(1000000, time.Minute, vc)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.Allow(ctx, "user1")
	}
}

// BenchmarkFixedWindow_MultiKey measures throughput across many keys.
func BenchmarkFixedWindow_MultiKey(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(1000, time.Minute, vc)
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.Allow(ctx, keys[i%len(keys)])
	}
}

// BenchmarkFixedWindow_Parallel measures concurrent throughput.
func BenchmarkFixedWindow_Parallel(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(1000000, time.Minute, vc)
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fw.Allow(ctx, fmt.Sprintf("user-%d", i%100))
			i++
		}
	})
}

// BenchmarkVirtualClock_Now measures VirtualClock.Now() overhead.
func BenchmarkVirtualClock_Now(b *testing.B) {
	vc := clock.NewVirtualClock(epoch)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			vc.Now()
		}
	})
}

// BenchmarkComparison runs all three algorithms side by side for easy comparison.
func BenchmarkComparison(b *testing.B) {
	algorithms := []struct {
		name string
		make func(clock.Clock) Limiter
	}{
		{"TokenBucket", func(c clock.Clock) Limiter { return NewTokenBucket(10000, time.Minute, 10000, c) }},
		{"SlidingWindow", func(c clock.Clock) Limiter { return NewSlidingWindow(10000, time.Minute, c) }},
		{"FixedWindow", func(c clock.Clock) Limiter { return NewFixedWindow(10000, time.Minute, c) }},
	}

	for _, alg := range algorithms {
		b.Run(alg.name+"/serial", func(b *testing.B) {
			vc := clock.NewVirtualClock(epoch)
			lim := alg.make(vc)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lim.Allow(ctx, "user1")
			}
		})

		b.Run(alg.name+"/parallel_100keys", func(b *testing.B) {
			vc := clock.NewVirtualClock(epoch)
			lim := alg.make(vc)
			ctx := context.Background()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					lim.Allow(ctx, fmt.Sprintf("user-%d", i%100))
					i++
				}
			})
		})
	}
}
