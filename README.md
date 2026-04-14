<p align="center">
  ![linux: tests](https://img.shields.io/badge/linux_tests-unknown-grey)
  ![linux: coverage](https://img.shields.io/badge/linux_coverage-unknown-grey)
  ![macos: tests](https://img.shields.io/badge/macOS_tests-unknown-grey)
  ![macos: coverage](https://img.shields.io/badge/macOS_coverage-unknown-grey)
  ![windows: tests](https://img.shields.io/badge/windows_tests-unknown-grey)
  ![windows: coverage](https://img.shields.io/badge/windows_coverage-unknown-grey)
  <br><img src="logo.png" alt="hpt logo" width="450">
</p>

> **High-precision replacements for Go's `time.Sleep`, `time.Ticker`, and `time.Timer` using OS-specific timing primitives.**

## Background

Go's standard `time.Ticker`, `time.Timer`, and `time.Sleep` route through the
runtime's internal timer system, which batches and coalesces timers to reduce
scheduling overhead. The result is typical resolution of **1–15ms** depending on
OS, load, and garbage collector activity. For many applications this is fine, but
for latency-sensitive workloads it's a problem:

- **Audio synthesis and DSP** need sample-accurate callbacks at 44.1/48/96 kHz
  (10–23us periods). A 5ms scheduling jitter produces audible glitches.
- **Game loops and render ticks** targeting 120–240 Hz (4–8ms frames) lose
  precision to timer coalescing, causing frame pacing stutter.
- **High-frequency trading and market data** require sub-millisecond wakeups for
  order management and event processing.
- **Real-time control systems** (robotics, hardware-in-the-loop simulation) rely
  on deterministic timing that can't tolerate scheduler jitter.

The root causes in Go are:

1. **Timer coalescing** — the runtime groups nearby timers into a single wake-up
   to amortize context switch costs, adding up to several milliseconds of delay.
2. **Goroutine scheduling** — a timer firing means "the goroutine becomes
   runnable," not "the goroutine runs now." It waits in the run queue behind
   other goroutines.
3. **GC stop-the-world pauses** — all goroutines are frozen during STW phases
   (typically <100us, but unbounded under memory pressure), adding
   unpredictable jitter to any Go-scheduled timer.

`hpt` addresses all three by bypassing the Go scheduler entirely:

- Each timer locks its goroutine to a dedicated **OS thread** and sleeps using
  the kernel's native high-precision primitives (`clock_nanosleep` on Linux,
  `kevent` with `NOTE_CRITICAL` on macOS, high-resolution waitable timers on
  Windows).
- Tick deadlines are computed as **absolute monotonic times** (`start + N*period`)
  rather than relative sleeps, preventing accumulated drift.
- When cgo is available, the `Ticker` loop runs on a raw **pthread** that is
  invisible to Go's garbage collector — making tick timing completely immune to
  GC pauses.

## Installation

```
go get github.com/daniel-sullivan/go-hpt
```

## Usage

### Sleep

```go
hpt.Sleep(500 * time.Microsecond)
```

### Ticker

```go
ticker := hpt.NewTicker(1 * time.Millisecond)
defer ticker.Stop()

for tick := range ticker.C {
    process(tick)
}
```

### Timer

```go
timer := hpt.NewTimer(10 * time.Millisecond)
<-timer.C

// Or call a function after a delay.
hpt.AfterFunc(5*time.Millisecond, func() {
    fmt.Println("fired")
})

// Channel shorthand.
<-hpt.After(1 * time.Millisecond)
```

### Monotonic Clock

Use the same high-precision clock the library uses internally:

```go
start := hpt.Now()
hpt.Sleep(d)
elapsed := hpt.Since(start)
```

## Platform Details

| | Linux | macOS | Windows |
|---|---|---|---|
| **Clock** | `clock_gettime(CLOCK_MONOTONIC)` | `mach_absolute_time` (cgo) / `clock_gettime` (no cgo) | `QueryPerformanceCounter` |
| **Sleep** | `clock_nanosleep(TIMER_ABSTIME)` | `kevent(NOTE_CRITICAL)` + spin | `CreateWaitableTimerExW(HIGH_RESOLUTION)` + spin |
| **Ticker** | pthread (cgo) / `LockOSThread` (no cgo) | pthread (cgo) / `LockOSThread` (no cgo) | `LockOSThread` |

When cgo is available (the default with a C compiler), the Ticker runs on a
dedicated pthread immune to GC pauses. With `CGO_ENABLED=0` or when
cross-compiling, it falls back to a goroutine with `runtime.LockOSThread` — still
far more precise than stdlib, but subject to GC jitter. Windows always uses the
pure-Go path.

## Benchmark Results

> **Note:** These benchmarks run on GitHub Actions shared runners, which are
> virtualized and subject to noisy-neighbor effects. Results may vary between
> runs. For precise measurements, run `go test -run TestPrecisionReport -v`
> on your own hardware.

<!-- BENCHMARK_RESULTS_START -->

_No benchmark data yet. Push to master to generate._

<!-- BENCHMARK_RESULTS_END -->

## Caveats

- **Thread consumption** — each active `Ticker`, `Timer`, or `Sleep` consumes a
  dedicated OS thread. Don't create thousands of concurrent hpt timers. This
  package is for a small number of high-precision timing sources, not
  general-purpose scheduling.

- **Overshoot, not undershoot** — the library guarantees it will never return
  *before* the requested deadline. A small overshoot of a few clock cycles is
  expected. Use `hpt.Now()` / `hpt.Since()` to measure with the same monotonic
  clock the sleep primitives use.

- **GC and the channel** — with cgo, the pthread fires ticks precisely, but the
  Go goroutine forwarding them to the channel can still be briefly paused by GC.
  The *tick timing* is GC-immune; the *channel delivery* has minimal GC jitter.

## License

MIT — see [LICENSE](LICENSE).