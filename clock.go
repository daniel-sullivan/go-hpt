package hpt

import "time"

// Now returns the current monotonic clock reading as nanoseconds since an
// arbitrary epoch. This is the same clock used internally by [Sleep],
// [Ticker], and [Timer] for deadline computation.
//
// Use this to measure elapsed time with the same precision as the timing
// primitives in this package:
//
//	start := hpt.Now()
//	hpt.Sleep(d)
//	elapsed := hpt.Since(start) // measured with our clock, not time.Now()
func Now() int64 {
	return monotonicNow()
}

// Since returns the nanoseconds elapsed since the given monotonic clock
// reading (as returned by [Now]).
func Since(start int64) time.Duration {
	return time.Duration(monotonicNow() - start)
}
