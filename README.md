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
  <td><code>58.732µs</code></td>
  <td><code>973.334µs</code></td>
  <td><b>16.6x</b></td>
  <td><code>59.707µs</code></td>
  <td><code>964.487µs</code></td>
  <td><b>16.2x</b></td>
  <td><code>127ns</code></td>
  <td><code>98.672µs</code></td>
  <td><b>776.9x</b></td>
  <td><code>11.223µs</code></td>
  <td><code>98.626µs</code></td>
  <td><b>8.8x</b></td>
  <td><code>435.362µs</code></td>
  <td><code>455.777µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>68.71µs</code></td>
  <td><code>572.923µs</code></td>
  <td><b>8.3x</b></td>
  <td><code>62.443µs</code></td>
  <td><code>563.593µs</code></td>
  <td><b>9.0x</b></td>
  <td><code>36.585µs</code></td>
  <td><code>226.733µs</code></td>
  <td><b>6.2x</b></td>
  <td><code>100.167µs</code></td>
  <td><code>242.523µs</code></td>
  <td><b>2.4x</b></td>
  <td><code>267.38µs</code></td>
  <td><code>528.839µs</code></td>
  <td><b>2.0x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>73.359µs</code></td>
  <td><code>75.358µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>63.515µs</code></td>
  <td><code>65.688µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>213.257µs</code></td>
  <td><code>549.137µs</code></td>
  <td><b>2.6x</b></td>
  <td><code>218.606µs</code></td>
  <td><code>492.894µs</code></td>
  <td><b>2.3x</b></td>
  <td><code>397.799µs</code></td>
  <td><code>531.736µs</code></td>
  <td><b>1.3x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>88.42µs</code></td>
  <td><code>184.828µs</code></td>
  <td><b>2.1x</b></td>
  <td><code>91.363µs</code></td>
  <td><code>162.293µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>281.193µs</code></td>
  <td><code>1.512953ms</code></td>
  <td><b>5.4x</b></td>
  <td><code>705.748µs</code></td>
  <td><code>1.64895ms</code></td>
  <td><b>2.3x</b></td>
  <td><code>262.587µs</code></td>
  <td><code>296.238µs</code></td>
  <td><b>1.1x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>8.342µs</code></td>
  <td><code>86.684µs</code></td>
  <td>—</td>
  <td><code>2.455µs</code></td>
  <td><code>73.227µs</code></td>
  <td>—</td>
  <td><code>3.625µs</code></td>
  <td><code>8.75µs</code></td>
  <td>—</td>
  <td><code>24µs</code></td>
  <td><code>72µs</code></td>
  <td>—</td>
  <td><code>55µs</code></td>
  <td><code>98.2µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>7.456µs</code></td>
  <td><code>109.122µs</code></td>
  <td><b>14.6x</b></td>
  <td><code>3.153µs</code></td>
  <td><code>88.767µs</code></td>
  <td><b>28.2x</b></td>
  <td><code>14.462µs</code></td>
  <td><code>81.149µs</code></td>
  <td><b>5.6x</b></td>
  <td><code>141.969µs</code></td>
  <td><code>113.3µs</code></td>
  <td><b>0.8x</b></td>
  <td><code>143.174µs</code></td>
  <td><code>206.33µs</code></td>
  <td><b>1.4x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>16.891µs</code></td>
  <td><code>128.983µs</code></td>
  <td>—</td>
  <td><code>6.626µs</code></td>
  <td><code>85.526µs</code></td>
  <td>—</td>
  <td><code>43.916µs</code></td>
  <td><code>220.083µs</code></td>
  <td>—</td>
  <td><code>680µs</code></td>
  <td><code>358µs</code></td>
  <td>—</td>
  <td><code>541.5µs</code></td>
  <td><code>562.6µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>19.336µs</code></td>
  <td><code>982.831µs</code></td>
  <td><b>50.8x</b></td>
  <td><code>20.693µs</code></td>
  <td><code>981.847µs</code></td>
  <td><b>47.4x</b></td>
  <td><code>120.25µs</code></td>
  <td><code>1.050166ms</code></td>
  <td><b>8.7x</b></td>
  <td><code>1.7ms</code></td>
  <td><code>470µs</code></td>
  <td><b>0.3x</b></td>
  <td><code>594.4µs</code></td>
  <td><code>594µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>37.254µs</code></td>
  <td><code>999.308µs</code></td>
  <td>—</td>
  <td><code>47.79µs</code></td>
  <td><code>987.597µs</code></td>
  <td>—</td>
  <td><code>999.375µs</code></td>
  <td><code>12.592917ms</code></td>
  <td>—</td>
  <td><code>12.053ms</code></td>
  <td><code>1.359ms</code></td>
  <td>—</td>
  <td><code>885.9µs</code></td>
  <td><code>990.6µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>1.025068ms</code></td>
  <td><code>31.037032ms</code></td>
  <td>—</td>
  <td><code>84.265µs</code></td>
  <td><code>26.702754ms</code></td>
  <td>—</td>
  <td><code>1.012084ms</code></td>
  <td><code>20.187042ms</code></td>
  <td>—</td>
  <td><code>16.026ms</code></td>
  <td><code>1.56ms</code></td>
  <td>—</td>
  <td><code>430.5µs</code></td>
  <td><code>550.9µs</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>78.254µs</code></td>
  <td><code>72.738µs</code></td>
  <td><b>0.9x</b></td>
  <td><code>66.809µs</code></td>
  <td><code>66.667µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>17.724µs</code></td>
  <td><code>275.081µs</code></td>
  <td><b>15.5x</b></td>
  <td><code>278.838µs</code></td>
  <td><code>420.987µs</code></td>
  <td><b>1.5x</b></td>
  <td><code>534.891µs</code></td>
  <td><code>519.114µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>115.54µs</code></td>
  <td><code>213.6µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>74.893µs</code></td>
  <td><code>168.593µs</code></td>
  <td><b>2.3x</b></td>
  <td><code>435.64µs</code></td>
  <td><code>1.524988ms</code></td>
  <td><b>3.5x</b></td>
  <td><code>840.919µs</code></td>
  <td><code>1.198806ms</code></td>
  <td><b>1.4x</b></td>
  <td><code>375.96µs</code></td>
  <td><code>299.393µs</code></td>
  <td><b>0.8x</b></td>
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