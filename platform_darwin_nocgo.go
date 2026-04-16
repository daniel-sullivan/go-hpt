//go:build darwin && !cgo

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
	for {
		remaining := deadline - monotonicNow()
		if remaining <= 0 {
			return
		}

		if remaining < 100_000 { // 100us: busy-spin
			spinUntil(deadline)
			return
		}

		// kevent with EVFILT_TIMER for kernel-level precision.
		kq, err := unix.Kqueue()
		if err != nil {
			spinUntil(deadline)
			return
		}

		sleepNs := remaining - 50_000
		if sleepNs < 0 {
			sleepNs = remaining / 2
		}

		kevt := unix.Kevent_t{
			Ident:  0,
			Filter: unix.EVFILT_TIMER,
			Flags:  unix.EV_ADD | unix.EV_ONESHOT | unix.EV_ENABLE,
			Fflags: unix.NOTE_NSECONDS | unix.NOTE_CRITICAL,
			Data:   sleepNs,
		}
		events := make([]unix.Kevent_t, 1)
		_, _ = unix.Kevent(kq, []unix.Kevent_t{kevt}, events, nil)
		_ = unix.Close(kq)

		spinUntil(deadline)
		return
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
