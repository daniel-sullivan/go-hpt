package hpt

import (
	"log/slog"
	"sync/atomic"
)

const defaultMaxThreads = 100

var (
	activeThreads atomic.Int64
	maxThreads    atomic.Int64
)

func init() {
	maxThreads.Store(defaultMaxThreads)
}

// SetMaxThreads sets the warning threshold for concurrent hpt threads.
// When the number of active Ticker, Timer, and Sleep threads exceeds n,
// a warning is logged via slog. Set to 0 to disable warnings.
//
// The default is 100, which is generous for typical use (1–5 tickers)
// but will catch runaway leaks before they exhaust OS threads.
func SetMaxThreads(n int) {
	maxThreads.Store(int64(n))
}

// ActiveThreads returns the current number of active hpt threads
// (Tickers, Timers, and Sleeps).
func ActiveThreads() int {
	return int(activeThreads.Load())
}

func threadStarted() {
	n := activeThreads.Add(1)
	if maxAllowedThreads := maxThreads.Load(); maxAllowedThreads > 0 && n > maxAllowedThreads {
		slog.Warn("hpt: active thread count exceeds limit",
			"active", n,
			"limit", maxAllowedThreads,
		)
	}
}

func threadStopped() {
	activeThreads.Add(-1)
}
