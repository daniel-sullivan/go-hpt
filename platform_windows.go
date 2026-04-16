//go:build windows

package hpt

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")
	procCreateWaitableTimerExW    = kernel32.NewProc("CreateWaitableTimerExW")
	procSetWaitableTimerEx        = kernel32.NewProc("SetWaitableTimerEx")
	procWaitForSingleObject       = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
	procNtSetTimerResolution      = ntdll.NewProc("NtSetTimerResolution")

	qpcFreq      int64
	highResAvail bool
	initOnce     sync.Once
)

const (
	createWaitableTimerHighResolution = 0x00000002
	timerAllAccess                    = 0x1F0003
	infinite                          = 0xFFFFFFFF
)

func initPlatform() {
	initOnce.Do(func() {
		procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&qpcFreq)))

		// Probe for high-resolution waitable timer support (Windows 10 1803+).
		h, _, _ := procCreateWaitableTimerExW.Call(0, 0, createWaitableTimerHighResolution, timerAllAccess)
		if h != 0 {
			highResAvail = true
			procCloseHandle.Call(h)
		}

		if !highResAvail {
			// Fallback: request 0.5ms system timer resolution via NtSetTimerResolution.
			// Resolution is in 100ns units: 5000 = 0.5ms.
			var cur uint32
			procNtSetTimerResolution.Call(5000, 1, uintptr(unsafe.Pointer(&cur)))
		}
	})
}

func monotonicNow() int64 {
	initPlatform()
	var count int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&count)))
	return qpcToNanos(count)
}

// qpcToNanos converts QPC ticks to nanoseconds without intermediate overflow.
func qpcToNanos(count int64) int64 {
	whole := count / qpcFreq
	frac := count % qpcFreq
	return whole*1_000_000_000 + frac*1_000_000_000/qpcFreq
}

func sleepUntil(deadline int64) {
	initPlatform()
	if highResAvail {
		sleepUntilHighRes(deadline)
	} else {
		sleepUntilFallback(deadline)
	}
}

func sleepUntilHighRes(deadline int64) {
	remaining := deadline - monotonicNow()
	if remaining <= 0 {
		return
	}

	h, _, _ := procCreateWaitableTimerExW.Call(0, 0, createWaitableTimerHighResolution, timerAllAccess)
	if h == 0 {
		sleepUntilFallback(deadline)
		return
	}
	defer procCloseHandle.Call(h)

	// SetWaitableTimerEx takes time in 100ns units, negative = relative.
	dueTime := -(remaining / 100)
	if dueTime == 0 {
		dueTime = -1
	}
	procSetWaitableTimerEx.Call(h, uintptr(unsafe.Pointer(&dueTime)), 0, 0, 0, 0, 0)
	procWaitForSingleObject.Call(h, infinite)

	// Busy-spin for any remainder.
	spinUntil(deadline)
}

func sleepUntilFallback(deadline int64) {
	for {
		remaining := deadline - monotonicNow()
		if remaining <= 0 {
			return
		}
		if remaining > 2_000_000 { // > 2ms
			time.Sleep(time.Duration(remaining - 1_500_000))
		} else {
			spinUntil(deadline)
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
