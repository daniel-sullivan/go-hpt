<p align="center">
  <img src="https://img.shields.io/badge/linux_tests-passing-brightgreen" alt="linux: tests">
  <img src="https://img.shields.io/badge/linux_coverage-46.1%25-orange" alt="linux: coverage">
  <img src="https://img.shields.io/badge/macOS_tests-passing-brightgreen" alt="macos: tests">
  <img src="https://img.shields.io/badge/macOS_coverage-45.9%25-orange" alt="macos: coverage">
  <img src="https://img.shields.io/badge/windows_tests-failing-red" alt="windows: tests">
  <img src="https://img.shields.io/badge/windows_coverage-unknown-grey" alt="windows: coverage">
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

Results below were measured on a dedicated local machine (Apple M4 Max, macOS).
Run benchmarks on your own hardware — no clone required:

```bash
go run github.com/daniel-sullivan/go-hpt/cmd/bench@latest
```

Pass `-json results.json` to emit a machine-readable report.

<!-- BENCHMARK_RESULTS_START -->

> Generated on 2026-04-16 &mdash; `mise run benchmarks`

Lower is better for all metrics. Impr. = how many times more precise `hpt` is vs `time`. Columns without "no cgo" use the default cgo build (pthread ticker, GC-immune).

<table>
<tr>
  <th colspan="2" rowspan="2"></th>
  <th colspan="3" align="center">macOS (arm64)</th>
  <th colspan="3" align="center">macOS (arm64) no cgo</th>
</tr>
<tr>
  <th><code>hpt</code></th>
  <th><code>time</code></th>
  <th>Impr.</th>
  <th><code>hpt</code></th>
  <th><code>time</code></th>
  <th>Impr.</th>
</tr>
<tr>
  <th rowspan="4" align="left">Sleep</th>
  <td><b>100µs</b></td>
  <td><code>1.185µs</code></td>
  <td><code>21.009µs</code></td>
  <td><b>17.7x</b></td>
  <td><code>104.537µs</code></td>
  <td><code>120.121µs</code></td>
  <td><b>1.1x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>5.704µs</code></td>
  <td><code>93.619µs</code></td>
  <td><b>16.4x</b></td>
  <td><code>162.25µs</code></td>
  <td><code>118.288µs</code></td>
  <td><b>0.7x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>5.364µs</code></td>
  <td><code>143.086µs</code></td>
  <td><b>26.7x</b></td>
  <td><code>27.581µs</code></td>
  <td><code>325.73µs</code></td>
  <td><b>11.8x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>12.473µs</code></td>
  <td><code>588.726µs</code></td>
  <td><b>47.2x</b></td>
  <td><code>106.258µs</code></td>
  <td><code>622.392µs</code></td>
  <td><b>5.9x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>3.708µs</code></td>
  <td><code>4.209µs</code></td>
  <td>—</td>
  <td><code>3µs</code></td>
  <td><code>6µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>10.537µs</code></td>
  <td><code>10.07µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>17.504µs</code></td>
  <td><code>20.232µs</code></td>
  <td><b>1.2x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>44.5µs</code></td>
  <td><code>42.792µs</code></td>
  <td>—</td>
  <td><code>53µs</code></td>
  <td><code>85µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>92.458µs</code></td>
  <td><code>87.417µs</code></td>
  <td><b>0.9x</b></td>
  <td><code>178µs</code></td>
  <td><code>178µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>275.458µs</code></td>
  <td><code>465µs</code></td>
  <td>—</td>
  <td><code>2.281ms</code></td>
  <td><code>941µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>-48.875µs</code></td>
  <td><code>109.667µs</code></td>
  <td>—</td>
  <td><code>27µs</code></td>
  <td><code>110µs</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>8.756µs</code></td>
  <td><code>143.843µs</code></td>
  <td><b>16.4x</b></td>
  <td><code>5.762µs</code></td>
  <td><code>136.42µs</code></td>
  <td><b>23.7x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>34.285µs</code></td>
  <td><code>634.364µs</code></td>
  <td><b>18.5x</b></td>
  <td><code>25.262µs</code></td>
  <td><code>585.256µs</code></td>
  <td><b>23.2x</b></td>
</tr>
</table>

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