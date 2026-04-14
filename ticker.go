package hpt

import (
	"sync"
	"time"
)

// Ticker holds a channel that delivers "ticks" at high-precision intervals.
// Unlike time.Ticker, it uses OS-specific timing primitives that bypass
// Go's runtime timer coalescing.
//
// When cgo is available (the default), the timing loop runs on a dedicated
// OS thread created via pthread, completely outside Go's runtime. This makes
// tick timing immune to GC stop-the-world pauses. When cgo is not available,
// the timing loop runs in a goroutine locked to an OS thread.
//
// Always call Stop when the Ticker is no longer needed to release resources.
type Ticker struct {
	C <-chan time.Time

	c       chan time.Time
	stopFn  func()
	mu      sync.Mutex
	stopped bool
}

// NewTicker returns a new Ticker that sends the current time on its channel
// at each tick. The period is specified by d. Ticks are computed as absolute
// deadlines (start + N*period) to prevent accumulated drift.
//
// Stop the ticker to release associated resources.
// Panics if d <= 0.
func NewTicker(d time.Duration) *Ticker {
	if d <= 0 {
		panic("hpt: non-positive interval for NewTicker")
	}
	c := make(chan time.Time, 1)
	t := &Ticker{
		C: c,
		c: c,
	}
	threadStarted()
	raw := startTickerLoop(d, c)
	t.stopFn = func() { raw(); threadStopped() }
	return t
}

// Stop turns off the ticker. After Stop, no more ticks will be sent.
// Stop does not close the channel, to prevent a concurrent goroutine
// reading from the channel from seeing an erroneous "tick".
func (t *Ticker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.stopped {
		t.stopped = true
		t.stopFn()
	}
}

// Reset stops the ticker and resets it to tick with the new period d.
// The next tick will arrive after the new period elapses from now.
// Panics if d <= 0.
func (t *Ticker) Reset(d time.Duration) {
	if d <= 0 {
		panic("hpt: non-positive interval for Ticker.Reset")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.stopped {
		t.stopFn()
	}
	t.stopped = false
	threadStarted()
	raw := startTickerLoop(d, t.c)
	t.stopFn = func() { raw(); threadStopped() }
}
