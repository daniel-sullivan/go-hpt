//go:build linux && !cgo

package hpt

import (
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

func monotonicNow() int64 {
	var ts unix.Timespec
	_ = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	return ts.Sec*1e9 + ts.Nsec
}

func sleepUntil(deadline int64) {
	ts := unix.Timespec{
		Sec:  deadline / 1e9,
		Nsec: deadline % 1e9,
	}
	for {
		err := unix.ClockNanosleep(unix.CLOCK_MONOTONIC, unix.TIMER_ABSTIME, &ts, nil)
		if err != unix.EINTR {
			return
		}
	}
}

func startTickerLoop(period time.Duration, c chan time.Time) (stop func()) {
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		start := monotonicNow()
		d := period.Nanoseconds()
		var tick int64

		for {
			tick++
			sleepUntil(start + tick*d)

			select {
			case <-done:
				return
			default:
			}

			select {
			case c <- time.Now():
			default:
			}
		}
	}()
	return func() { close(done) }
}
