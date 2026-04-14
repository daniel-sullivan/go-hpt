package hpt

import (
	"runtime"
	"time"
)

// Sleep pauses the current goroutine for at least the duration d using
// OS-level high-precision sleep primitives. Unlike time.Sleep, this locks
// the current goroutine to an OS thread and bypasses Go's timer coalescing.
//
// A negative or zero duration causes Sleep to return immediately.
//
// The calling goroutine's OS thread is locked for the duration of the sleep.
// Do not use this in high-concurrency scenarios with thousands of sleeping
// goroutines, as each consumes a real OS thread.
func Sleep(d time.Duration) {
	if d <= 0 {
		return
	}

	threadStarted()
	defer threadStopped()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	deadline := monotonicNow() + d.Nanoseconds()
	sleepUntil(deadline)
}
