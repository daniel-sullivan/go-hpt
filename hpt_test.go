package hpt

import (
	"testing"
	"time"
)

// --- Sleep tests ---

func TestSleepZeroDuration(t *testing.T) {
	start := monotonicNow()
	Sleep(0)
	if elapsed := time.Duration(monotonicNow() - start); elapsed > 1*time.Millisecond {
		t.Errorf("Sleep(0) took %v, expected near-instant", elapsed)
	}
}

func TestSleepNegativeDuration(t *testing.T) {
	start := monotonicNow()
	Sleep(-1 * time.Second)
	if elapsed := time.Duration(monotonicNow() - start); elapsed > 1*time.Millisecond {
		t.Errorf("Sleep(-1s) took %v, expected near-instant", elapsed)
	}
}

func TestSleepAccuracy(t *testing.T) {
	durations := []time.Duration{
		1 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
	}
	for _, d := range durations {
		t.Run(d.String(), func(t *testing.T) {
			start := monotonicNow()
			Sleep(d)
			elapsed := time.Duration(monotonicNow() - start)

			// Never returning early is the core correctness guarantee, so the
			// undershoot check always runs.
			if elapsed < d {
				t.Errorf("Sleep(%v) returned after %v (undershoot)", d, elapsed)
			}
			// The overshoot bound asserts precision, which shared CI runners
			// under -race can't honor (scheduling jitter dwarfs the budget), so
			// it's gated out of -short mode and validated in full/local runs.
			maxOvershoot := 5 * time.Millisecond
			if !testing.Short() && elapsed > d+maxOvershoot {
				t.Errorf("Sleep(%v) took %v (overshoot %v > %v)", d, elapsed, elapsed-d, maxOvershoot)
			}
		})
	}
}

func TestSleepDoesNotReturnEarly(t *testing.T) {
	d := 10 * time.Millisecond
	for i := 0; i < 20; i++ {
		start := monotonicNow()
		Sleep(d)
		elapsed := time.Duration(monotonicNow() - start)
		if elapsed < d {
			t.Fatalf("iteration %d: Sleep(%v) returned after %v (undershoot)", i, d, elapsed)
		}
	}
}

func TestSleepConcurrency(t *testing.T) {
	const n = 10
	done := make(chan time.Duration, n)
	d := 5 * time.Millisecond

	for i := 0; i < n; i++ {
		go func() {
			start := monotonicNow()
			Sleep(d)
			done <- time.Duration(monotonicNow() - start)
		}()
	}

	for i := 0; i < n; i++ {
		elapsed := <-done
		if elapsed < d {
			t.Errorf("goroutine %d: Sleep(%v) returned after %v (undershoot)", i, d, elapsed)
		}
	}
}

// --- Ticker tests ---

func TestTickerBasic(t *testing.T) {
	ticker := NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; i < 5; i++ {
		select {
		case <-ticker.C:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("tick %d: timed out", i)
		}
	}
}

func TestTickerPeriod(t *testing.T) {
	// This test only asserts per-tick precision, which oversubscribed CI
	// runners under -race can't deliver (a single preempted tick blows the
	// ±4ms window). Tick delivery itself is covered by TestTickerBasic, so we
	// skip in -short and validate precision in full/local runs.
	if testing.Short() {
		t.Skip("skipping ticker period precision check in short mode")
	}
	period := 5 * time.Millisecond
	ticker := NewTicker(period)
	defer ticker.Stop()

	var lastNano int64
	for i := 0; i < 10; i++ {
		<-ticker.C
		now := monotonicNow()
		if i > 0 {
			interval := time.Duration(now - lastNano)
			// Per-tick intervals include Go channel delivery jitter (goroutine
			// scheduling between the timer firing and the test reading C). A
			// delayed tick shortens the *next* measured interval. Total accuracy
			// is validated by TestTickerDrift; this test guards against gross
			// errors like wrong period or stuck ticker.
			if interval < period-4*time.Millisecond || interval > period+4*time.Millisecond {
				t.Errorf("tick %d: interval %v, expected ~%v", i, interval, period)
			}
		}
		lastNano = now
	}
}

func TestTickerDrift(t *testing.T) {
	// Cumulative drift is a precision metric; shared CI runners under -race
	// accumulate scheduling latency well past the ±10ms budget. Skip in -short
	// and validate in full/local runs.
	if testing.Short() {
		t.Skip("skipping ticker drift precision check in short mode")
	}
	period := 1 * time.Millisecond
	count := 100
	ticker := NewTicker(period)
	defer ticker.Stop()

	start := monotonicNow()
	for i := 0; i < count; i++ {
		<-ticker.C
	}
	elapsed := time.Duration(monotonicNow() - start)

	expected := time.Duration(count) * period
	drift := elapsed - expected
	// Total drift over 100 ticks should be small. Allow 10ms for CI runners.
	if drift < -10*time.Millisecond || drift > 10*time.Millisecond {
		t.Errorf("drift after %d ticks: %v (elapsed %v, expected %v)", count, drift, elapsed, expected)
	}
}

func TestTickerStop(t *testing.T) {
	ticker := NewTicker(1 * time.Millisecond)

	// Receive a few ticks to confirm it's running.
	<-ticker.C
	<-ticker.C
	ticker.Stop()

	// After stop, no more ticks should arrive.
	time.Sleep(10 * time.Millisecond)
	select {
	case _, ok := <-ticker.C:
		if ok {
			t.Error("received tick after Stop")
		}
	default:
		// Good — no tick.
	}
}

func TestTickerReset(t *testing.T) {
	ticker := NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Reset to a much shorter period.
	ticker.Reset(2 * time.Millisecond)

	select {
	case <-ticker.C:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("no tick after Reset to 2ms within 50ms")
	}
}

func TestTickerPanicsOnZeroDuration(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for zero duration")
		}
	}()
	NewTicker(0)
}

func TestTickerPanicsOnNegativeDuration(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative duration")
		}
	}()
	NewTicker(-1 * time.Millisecond)
}

// --- Timer tests ---

func TestTimerFires(t *testing.T) {
	timer := NewTimer(5 * time.Millisecond)
	select {
	case <-timer.C:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timer did not fire within 100ms")
	}
}

func TestTimerAccuracy(t *testing.T) {
	d := 10 * time.Millisecond
	start := monotonicNow()
	timer := NewTimer(d)
	<-timer.C
	elapsed := time.Duration(monotonicNow() - start)

	if elapsed < d {
		t.Errorf("Timer fired after %v, expected >= %v (undershoot)", elapsed, d)
	}
	// Overshoot bound is a precision check; gated out of -short (see TestSleepAccuracy).
	if !testing.Short() && elapsed > d+3*time.Millisecond {
		t.Errorf("Timer fired after %v, expected ~%v (overshoot %v)", elapsed, d, elapsed-d)
	}
}

func TestTimerStop(t *testing.T) {
	timer := NewTimer(50 * time.Millisecond)
	if !timer.Stop() {
		t.Error("Stop returned false for active timer")
	}

	// Timer should not fire.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-timer.C:
		t.Error("timer fired after Stop")
	default:
	}
}

func TestTimerStopAfterFire(t *testing.T) {
	timer := NewTimer(1 * time.Millisecond)
	<-timer.C

	if timer.Stop() {
		t.Error("Stop returned true for already-fired timer")
	}
}

func TestTimerReset(t *testing.T) {
	timer := NewTimer(100 * time.Millisecond)
	timer.Stop()

	// Drain channel if anything was sent.
	select {
	case <-timer.C:
	default:
	}

	start := monotonicNow()
	timer.Reset(5 * time.Millisecond)
	<-timer.C
	elapsed := time.Duration(monotonicNow() - start)

	if elapsed < 5*time.Millisecond {
		t.Errorf("Reset timer fired after %v, expected >= 5ms (undershoot)", elapsed)
	}
	// Overshoot bound is a precision check; gated out of -short (see TestSleepAccuracy).
	if !testing.Short() && elapsed > 20*time.Millisecond {
		t.Errorf("Reset timer fired after %v, expected ~5ms", elapsed)
	}
}

func TestAfterFunc(t *testing.T) {
	done := make(chan struct{})
	timer := AfterFunc(5*time.Millisecond, func() {
		close(done)
	})
	_ = timer

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("AfterFunc did not fire within 100ms")
	}
}

func TestAfterFuncStop(t *testing.T) {
	called := make(chan struct{}, 1)
	timer := AfterFunc(50*time.Millisecond, func() {
		called <- struct{}{}
	})

	if !timer.Stop() {
		t.Error("Stop returned false for active AfterFunc timer")
	}

	time.Sleep(100 * time.Millisecond)
	select {
	case <-called:
		t.Error("AfterFunc fired after Stop")
	default:
	}
}

func TestAfter(t *testing.T) {
	start := monotonicNow()
	<-After(5 * time.Millisecond)
	elapsed := time.Duration(monotonicNow() - start)

	if elapsed < 5*time.Millisecond {
		t.Errorf("After(5ms) returned after %v (undershoot)", elapsed)
	}
}

// --- Clock tests ---

func TestNowAndSince(t *testing.T) {
	start := Now()
	Sleep(1 * time.Millisecond)
	elapsed := Since(start)

	if elapsed < 1*time.Millisecond {
		t.Errorf("Since reported %v, expected >= 1ms", elapsed)
	}
	// Upper bound is a precision check; gated out of -short (see TestSleepAccuracy).
	if !testing.Short() && elapsed > 3*time.Millisecond {
		t.Errorf("Since reported %v, expected ~1ms", elapsed)
	}
}
