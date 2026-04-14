// Package hpt provides high-precision timing primitives that bypass Go's
// runtime timer coalescing. It mirrors the [time.Ticker], [time.Timer], and
// [time.Sleep] APIs but uses OS-specific high-resolution timing mechanisms.
//
// Go's standard timer system coalesces timers and routes them through the
// runtime scheduler, yielding typical resolution of 1–15ms. This package
// uses platform-native sleep primitives to achieve significantly better
// precision.
//
// # cgo vs Pure Go
//
// When cgo is available (the default on Linux and macOS), the [Ticker]
// timing loop runs on a dedicated pthread that is completely invisible to
// Go's garbage collector. This means tick timing is immune to GC
// stop-the-world pauses. [Sleep] and [Timer] also use native C sleep
// primitives (mach_wait_until on macOS, clock_nanosleep on Linux).
//
// When cgo is disabled (CGO_ENABLED=0, or cross-compiling), the package
// falls back to a pure-Go implementation that locks a goroutine to an OS
// thread. This is still much more precise than the standard library, but
// ticks may be delayed by GC pauses (typically <100µs on modern Go).
//
// # Platform Implementations
//
// With cgo (default):
//
//   - Linux: clock_nanosleep(CLOCK_MONOTONIC, TIMER_ABSTIME), pthread ticker.
//   - macOS: mach_wait_until (absolute Mach time), pthread ticker.
//
// Without cgo:
//
//   - Linux: clock_nanosleep via golang.org/x/sys/unix.
//   - macOS: kevent(EVFILT_TIMER, NOTE_NSECONDS|NOTE_CRITICAL) + spin tail.
//
// Both modes:
//
//   - Windows: CreateWaitableTimerExW with CREATE_WAITABLE_TIMER_HIGH_RESOLUTION
//     (Windows 10 1803+), falling back to NtSetTimerResolution for older versions.
//
// # Thread Usage
//
// Each active [Ticker], [Timer], or [Sleep] call consumes a dedicated OS
// thread. With cgo, the Ticker thread is a raw pthread; without cgo, it is
// a Go OS thread via [runtime.LockOSThread].
//
// Do not create thousands of concurrent hpt timers. This package is designed
// for a small number of high-precision timing sources (audio callbacks, game
// loops, high-frequency data processing), not for general-purpose scheduling.
//
// # Drift Prevention
//
// [Ticker] computes absolute deadlines as start + N*period rather than
// sleeping for a relative duration after each tick. This prevents accumulated
// drift regardless of how many ticks have elapsed.
package hpt
