<p align="center">
  <img src="https://img.shields.io/badge/linux_tests-unknown-grey" alt="linux: tests">
  <img src="https://img.shields.io/badge/linux_coverage-unknown-grey" alt="linux: coverage">
  <img src="https://img.shields.io/badge/macOS_tests-unknown-grey" alt="macos: tests">
  <img src="https://img.shields.io/badge/macOS_coverage-unknown-grey" alt="macos: coverage">
  <img src="https://img.shields.io/badge/windows_tests-unknown-grey" alt="windows: tests">
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

> **Note:** These benchmarks run on GitHub Actions shared runners, which are
> virtualized and subject to noisy-neighbor effects. Results may vary between
> runs. For precise measurements, run `go test -run TestPrecisionReport -v`
> on your own hardware.

<!-- BENCHMARK_RESULTS_START -->

> Auto-generated on 2026-04-14 by CI &mdash; [view workflow](../../actions/workflows/benchmarks.yml)

Lower is better for all metrics. Impr. = how many times more precise `hpt` is vs `time`. Columns without "no cgo" use the default cgo build (pthread ticker, GC-immune).

<table>
<tr>
  <th colspan="2" rowspan="2"></th>
  <th colspan="3" align="center">Linux (amd64)</th>
  <th colspan="3" align="center">Linux (amd64) no cgo</th>
  <th colspan="3" align="center">macOS (arm64)</th>
  <th colspan="3" align="center">macOS (arm64) no cgo</th>
  <th colspan="3" align="center">Windows (amd64)</th>
</tr>
<tr>
  <th><code>hpt</code></th>
  <th><code>time</code></th>
  <th>Impr.</th>
  <th><code>hpt</code></th>
  <th><code>time</code></th>
  <th>Impr.</th>
  <th><code>hpt</code></th>
  <th><code>time</code></th>
  <th>Impr.</th>
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
  <td><code>64.459µs</code></td>
  <td><code>973.335µs</code></td>
  <td><b>15.1x</b></td>
  <td><code>58.511µs</code></td>
  <td><code>965.575µs</code></td>
  <td><b>16.5x</b></td>
  <td><code>17.299µs</code></td>
  <td><code>79.207µs</code></td>
  <td><b>4.6x</b></td>
  <td><code>946ns</code></td>
  <td><code>81.075µs</code></td>
  <td><b>85.7x</b></td>
  <td><code>498.603µs</code></td>
  <td><code>472.166µs</code></td>
  <td><b>0.9x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>68.667µs</code></td>
  <td><code>573.453µs</code></td>
  <td><b>8.4x</b></td>
  <td><code>63.111µs</code></td>
  <td><code>566.916µs</code></td>
  <td><b>9.0x</b></td>
  <td><code>98.478µs</code></td>
  <td><code>314.287µs</code></td>
  <td><b>3.2x</b></td>
  <td><code>64.735µs</code></td>
  <td><code>136.98µs</code></td>
  <td><b>2.1x</b></td>
  <td><code>358.309µs</code></td>
  <td><code>476.678µs</code></td>
  <td><b>1.3x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>69.404µs</code></td>
  <td><code>72.92µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>61.707µs</code></td>
  <td><code>71.265µs</code></td>
  <td><b>1.2x</b></td>
  <td><code>269.62µs</code></td>
  <td><code>569.735µs</code></td>
  <td><b>2.1x</b></td>
  <td><code>289.169µs</code></td>
  <td><code>633.944µs</code></td>
  <td><b>2.2x</b></td>
  <td><code>435.976µs</code></td>
  <td><code>543.092µs</code></td>
  <td><b>1.2x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>99.311µs</code></td>
  <td><code>161.672µs</code></td>
  <td><b>1.6x</b></td>
  <td><code>72.848µs</code></td>
  <td><code>135.932µs</code></td>
  <td><b>1.9x</b></td>
  <td><code>1.447788ms</code></td>
  <td><code>1.608308ms</code></td>
  <td><b>1.1x</b></td>
  <td><code>1.051338ms</code></td>
  <td><code>1.754206ms</code></td>
  <td><b>1.7x</b></td>
  <td><code>294.031µs</code></td>
  <td><code>264.232µs</code></td>
  <td><b>0.9x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>2.98µs</code></td>
  <td><code>88.784µs</code></td>
  <td>—</td>
  <td><code>3.875µs</code></td>
  <td><code>70.673µs</code></td>
  <td>—</td>
  <td><code>28.042µs</code></td>
  <td><code>72.708µs</code></td>
  <td>—</td>
  <td><code>13µs</code></td>
  <td><code>139µs</code></td>
  <td>—</td>
  <td><code>62.5µs</code></td>
  <td><code>108.9µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>4.345µs</code></td>
  <td><code>99.952µs</code></td>
  <td><b>23.0x</b></td>
  <td><code>17.08µs</code></td>
  <td><code>88.599µs</code></td>
  <td><b>5.2x</b></td>
  <td><code>72.192µs</code></td>
  <td><code>109.851µs</code></td>
  <td><b>1.5x</b></td>
  <td><code>186.519µs</code></td>
  <td><code>158.647µs</code></td>
  <td><b>0.9x</b></td>
  <td><code>161.256µs</code></td>
  <td><code>209.302µs</code></td>
  <td><b>1.3x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>12.514µs</code></td>
  <td><code>107.199µs</code></td>
  <td>—</td>
  <td><code>24.261µs</code></td>
  <td><code>93.167µs</code></td>
  <td>—</td>
  <td><code>302.125µs</code></td>
  <td><code>350.75µs</code></td>
  <td>—</td>
  <td><code>426µs</code></td>
  <td><code>357µs</code></td>
  <td>—</td>
  <td><code>549.5µs</code></td>
  <td><code>572.8µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>39.742µs</code></td>
  <td><code>969.311µs</code></td>
  <td><b>24.4x</b></td>
  <td><code>454.933µs</code></td>
  <td><code>986.25µs</code></td>
  <td><b>2.2x</b></td>
  <td><code>438.75µs</code></td>
  <td><code>507.417µs</code></td>
  <td><b>1.2x</b></td>
  <td><code>1ms</code></td>
  <td><code>542µs</code></td>
  <td><b>0.5x</b></td>
  <td><code>600.9µs</code></td>
  <td><code>600.8µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>46.668µs</code></td>
  <td><code>999.279µs</code></td>
  <td>—</td>
  <td><code>2.455565ms</code></td>
  <td><code>998.578µs</code></td>
  <td>—</td>
  <td><code>486.458µs</code></td>
  <td><code>552.625µs</code></td>
  <td>—</td>
  <td><code>23.254ms</code></td>
  <td><code>1.612ms</code></td>
  <td>—</td>
  <td><code>876.8µs</code></td>
  <td><code>635.6µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>1.027466ms</code></td>
  <td><code>34.354596ms</code></td>
  <td>—</td>
  <td><code>1.083738ms</code></td>
  <td><code>26.579038ms</code></td>
  <td>—</td>
  <td><code>942.458µs</code></td>
  <td><code>304.667µs</code></td>
  <td>—</td>
  <td><code>44.283ms</code></td>
  <td><code>1.517ms</code></td>
  <td>—</td>
  <td><code>190.5µs</code></td>
  <td><code>347.3µs</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>74.082µs</code></td>
  <td><code>73.964µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>66.324µs</code></td>
  <td><code>69.875µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>168.156µs</code></td>
  <td><code>326.624µs</code></td>
  <td><b>1.9x</b></td>
  <td><code>193.522µs</code></td>
  <td><code>392.66µs</code></td>
  <td><b>2.0x</b></td>
  <td><code>515.822µs</code></td>
  <td><code>524.562µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>98.947µs</code></td>
  <td><code>149.217µs</code></td>
  <td><b>1.5x</b></td>
  <td><code>74.336µs</code></td>
  <td><code>134.602µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>325.807µs</code></td>
  <td><code>1.836912ms</code></td>
  <td><b>5.6x</b></td>
  <td><code>806.946µs</code></td>
  <td><code>2.010489ms</code></td>
  <td><b>2.5x</b></td>
  <td><code>374.834µs</code></td>
  <td><code>321.982µs</code></td>
  <td><b>0.9x</b></td>
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