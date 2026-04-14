package hpt

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Timer represents a single event. When the Timer expires, the current time
// will be sent on C. A Timer must be created with NewTimer or AfterFunc.
//
// Each Timer consumes a dedicated OS thread (via runtime.LockOSThread)
// until it fires or is stopped.
type Timer struct {
	C <-chan time.Time

	c       chan time.Time
	f       func() // non-nil for AfterFunc timers
	stopped atomic.Bool
	fired   atomic.Bool
	mu      sync.Mutex
	resetCh chan time.Duration
	done    chan struct{}
}

// NewTimer creates a new Timer that will send the current time on its
// channel after at least duration d using high-precision OS primitives.
func NewTimer(d time.Duration) *Timer {
	c := make(chan time.Time, 1)
	done := make(chan struct{})
	resetCh := make(chan time.Duration, 1)
	t := &Timer{
		C:       c,
		c:       c,
		resetCh: resetCh,
		done:    done,
	}
	go t.run(d, c, nil, done, resetCh)
	return t
}

// AfterFunc waits for the duration to elapse and then calls f in its own
// goroutine. It returns a Timer that can be used to cancel the call using
// its Stop method. The returned Timer's C field is nil.
func AfterFunc(d time.Duration, f func()) *Timer {
	done := make(chan struct{})
	resetCh := make(chan time.Duration, 1)
	t := &Timer{
		f:       f,
		resetCh: resetCh,
		done:    done,
	}
	go t.run(d, nil, f, done, resetCh)
	return t
}

// After waits for the duration to elapse and then sends the current time
// on the returned channel. It is equivalent to NewTimer(d).C.
func After(d time.Duration) <-chan time.Time {
	return NewTimer(d).C
}

// Stop prevents the Timer from firing. It returns true if the call stops
// the timer, false if the timer has already expired or been stopped.
func (t *Timer) Stop() bool {
	wasActive := !t.stopped.Swap(true) && !t.fired.Load()
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return wasActive
}

// Reset changes the timer to expire after duration d. It returns true if
// the timer had been active, false if the timer had expired or been stopped.
//
// A timer must be stopped or expired and its channel drained before Reset
// is called.
func (t *Timer) Reset(d time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	wasActive := !t.fired.Load() && !t.stopped.Load()

	// Restart the timer.
	t.stopped.Store(false)
	t.fired.Store(false)

	// If the previous goroutine is still alive, signal a reset.
	select {
	case t.resetCh <- d:
	default:
	}

	// If the previous goroutine finished, start a new one.
	select {
	case <-t.done:
		done := make(chan struct{})
		resetCh := make(chan time.Duration, 1)
		t.done = done
		t.resetCh = resetCh
		go t.run(d, t.c, t.f, done, resetCh)
	default:
	}

	return wasActive
}

// run is the timer goroutine. It captures c, f, done, and resetCh as local
// values to avoid races with Stop/Reset which modify the struct under the mutex.
func (t *Timer) run(d time.Duration, c chan time.Time, f func(), done <-chan struct{}, resetCh <-chan time.Duration) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		deadline := monotonicNow() + d.Nanoseconds()
		sleepUntil(deadline)

		// Check for stop or reset.
		select {
		case <-done:
			return
		case newD := <-resetCh:
			d = newD
			continue
		default:
		}

		if t.stopped.Load() {
			return
		}

		t.fired.Store(true)
		if f != nil {
			go f()
		} else if c != nil {
			select {
			case c <- time.Now():
			default:
			}
		}
		return
	}
}
