package clock

import (
	"sync"
	"testing"
	"time"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestVirtualClock_Now(t *testing.T) {
	vc := NewVirtualClock(epoch)
	if got := vc.Now(); !got.Equal(epoch) {
		t.Errorf("Now() = %v, want %v", got, epoch)
	}
}

func TestVirtualClock_Advance(t *testing.T) {
	vc := NewVirtualClock(epoch)
	vc.Advance(5 * time.Minute)

	want := epoch.Add(5 * time.Minute)
	if got := vc.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance = %v, want %v", got, want)
	}
}

func TestVirtualClock_AdvanceMultiple(t *testing.T) {
	vc := NewVirtualClock(epoch)
	vc.Advance(1 * time.Hour)
	vc.Advance(30 * time.Minute)

	want := epoch.Add(90 * time.Minute)
	if got := vc.Now(); !got.Equal(want) {
		t.Errorf("Now() after multiple Advance = %v, want %v", got, want)
	}
}

func TestVirtualClock_AdvanceNegativePanics(t *testing.T) {
	vc := NewVirtualClock(epoch)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on negative advance")
		}
	}()
	vc.Advance(-1 * time.Second)
}

func TestVirtualClock_Set(t *testing.T) {
	vc := NewVirtualClock(epoch)
	target := epoch.Add(24 * time.Hour)
	vc.Set(target)

	if got := vc.Now(); !got.Equal(target) {
		t.Errorf("Now() after Set = %v, want %v", got, target)
	}
}

func TestVirtualClock_SetPastPanics(t *testing.T) {
	vc := NewVirtualClock(epoch)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on setting time to the past")
		}
	}()
	vc.Set(epoch.Add(-1 * time.Hour))
}

func TestVirtualClock_Since(t *testing.T) {
	vc := NewVirtualClock(epoch)
	start := vc.Now()
	vc.Advance(10 * time.Second)

	got := vc.Since(start)
	want := 10 * time.Second
	if got != want {
		t.Errorf("Since() = %v, want %v", got, want)
	}
}

func TestVirtualClock_After_FiresOnAdvance(t *testing.T) {
	vc := NewVirtualClock(epoch)
	ch := vc.After(5 * time.Second)

	// Should not fire yet.
	select {
	case <-ch:
		t.Fatal("After() fired before advance")
	default:
	}

	// Advance past the deadline.
	vc.Advance(5 * time.Second)

	select {
	case got := <-ch:
		want := epoch.Add(5 * time.Second)
		if !got.Equal(want) {
			t.Errorf("After() sent %v, want %v", got, want)
		}
	default:
		t.Fatal("After() did not fire after advance")
	}
}

func TestVirtualClock_After_FiresOnSet(t *testing.T) {
	vc := NewVirtualClock(epoch)
	ch := vc.After(1 * time.Hour)

	target := epoch.Add(2 * time.Hour)
	vc.Set(target)

	select {
	case <-ch:
		// OK
	default:
		t.Fatal("After() did not fire after Set()")
	}
}

func TestVirtualClock_After_ZeroDuration(t *testing.T) {
	vc := NewVirtualClock(epoch)
	ch := vc.After(0)

	select {
	case <-ch:
		// OK — fires immediately.
	default:
		t.Fatal("After(0) should fire immediately")
	}
}

func TestVirtualClock_After_MultipleWaiters(t *testing.T) {
	vc := NewVirtualClock(epoch)
	ch1 := vc.After(1 * time.Second)
	ch2 := vc.After(5 * time.Second)
	ch3 := vc.After(10 * time.Second)

	// Advance to 5s — should fire ch1 and ch2 but not ch3.
	vc.Advance(5 * time.Second)

	select {
	case <-ch1:
	default:
		t.Error("ch1 should have fired")
	}
	select {
	case <-ch2:
	default:
		t.Error("ch2 should have fired")
	}
	select {
	case <-ch3:
		t.Error("ch3 should NOT have fired")
	default:
	}

	// Advance to 10s — ch3 fires.
	vc.Advance(5 * time.Second)
	select {
	case <-ch3:
	default:
		t.Error("ch3 should have fired after second advance")
	}
}

func TestVirtualClock_ConcurrentAccess(t *testing.T) {
	vc := NewVirtualClock(epoch)

	var wg sync.WaitGroup
	// Readers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = vc.Now()
			_ = vc.Since(epoch)
		}()
	}
	// Writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			vc.Advance(1 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Just verify no panic or race.
	got := vc.Now()
	want := epoch.Add(100 * time.Millisecond)
	if !got.Equal(want) {
		t.Errorf("after concurrent ops, Now() = %v, want %v", got, want)
	}
}

func TestRealClock_Implements_Clock(t *testing.T) {
	var _ Clock = NewRealClock()
}

func TestVirtualClock_Implements_Clock(t *testing.T) {
	var _ Clock = NewVirtualClock(time.Now())
}
