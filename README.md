<p align="center">
  <img src="https://img.shields.io/badge/linux_tests-passing-brightgreen" alt="linux: tests">
  <img src="https://img.shields.io/badge/linux_coverage-78.0%25-yellow" alt="linux: coverage">
  <img src="https://img.shields.io/badge/macOS_tests-failing-red" alt="macos: tests">
  <img src="https://img.shields.io/badge/macOS_coverage-unknown-grey" alt="macos: coverage">
  <img src="https://img.shields.io/badge/windows_tests-passing-brightgreen" alt="windows: tests">
  <img src="https://img.shields.io/badge/windows_coverage-85.0%25-brightgreen" alt="windows: coverage">
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

> Auto-generated on 2026-04-16 by CI &mdash; [view workflow](../../actions/workflows/benchmarks.yml)

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
  <td><code>66.151µs</code></td>
  <td><code>970.685µs</code></td>
  <td><b>14.7x</b></td>
  <td><code>60.546µs</code></td>
  <td><code>963.954µs</code></td>
  <td><b>15.9x</b></td>
  <td><code>158ns</code></td>
  <td><code>72.013µs</code></td>
  <td><b>455.8x</b></td>
  <td><code>10.08µs</code></td>
  <td><code>96.372µs</code></td>
  <td><b>9.6x</b></td>
  <td><code>437.605µs</code></td>
  <td><code>443.658µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>67.922µs</code></td>
  <td><code>570.471µs</code></td>
  <td><b>8.4x</b></td>
  <td><code>61.829µs</code></td>
  <td><code>564.713µs</code></td>
  <td><b>9.1x</b></td>
  <td><code>19.058µs</code></td>
  <td><code>171.566µs</code></td>
  <td><b>9.0x</b></td>
  <td><code>103.36µs</code></td>
  <td><code>252.447µs</code></td>
  <td><b>2.4x</b></td>
  <td><code>344.694µs</code></td>
  <td><code>538.204µs</code></td>
  <td><b>1.6x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>69.907µs</code></td>
  <td><code>71.272µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>62.985µs</code></td>
  <td><code>64.51µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>80.886µs</code></td>
  <td><code>308.982µs</code></td>
  <td><b>3.8x</b></td>
  <td><code>160.34µs</code></td>
  <td><code>478.308µs</code></td>
  <td><b>3.0x</b></td>
  <td><code>349.213µs</code></td>
  <td><code>520.634µs</code></td>
  <td><b>1.5x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>101.958µs</code></td>
  <td><code>167.393µs</code></td>
  <td><b>1.6x</b></td>
  <td><code>91.941µs</code></td>
  <td><code>169.95µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>436.208µs</code></td>
  <td><code>1.789436ms</code></td>
  <td><b>4.1x</b></td>
  <td><code>913.48µs</code></td>
  <td><code>1.849052ms</code></td>
  <td><b>2.0x</b></td>
  <td><code>255.674µs</code></td>
  <td><code>258.308µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>3.813µs</code></td>
  <td><code>89.834µs</code></td>
  <td>—</td>
  <td><code>2.984µs</code></td>
  <td><code>68.503µs</code></td>
  <td>—</td>
  <td><code>4.292µs</code></td>
  <td><code>21.208µs</code></td>
  <td>—</td>
  <td><code>30µs</code></td>
  <td><code>51µs</code></td>
  <td>—</td>
  <td><code>53.6µs</code></td>
  <td><code>90.6µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>4.63µs</code></td>
  <td><code>102.892µs</code></td>
  <td><b>22.2x</b></td>
  <td><code>9.417µs</code></td>
  <td><code>75.104µs</code></td>
  <td><b>8.0x</b></td>
  <td><code>88.789µs</code></td>
  <td><code>238.534µs</code></td>
  <td><b>2.7x</b></td>
  <td><code>94.205µs</code></td>
  <td><code>100.383µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>145.203µs</code></td>
  <td><code>195.523µs</code></td>
  <td><b>1.3x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>12.158µs</code></td>
  <td><code>110.903µs</code></td>
  <td>—</td>
  <td><code>20.229µs</code></td>
  <td><code>74.664µs</code></td>
  <td>—</td>
  <td><code>296.125µs</code></td>
  <td><code>702.959µs</code></td>
  <td>—</td>
  <td><code>371µs</code></td>
  <td><code>365µs</code></td>
  <td>—</td>
  <td><code>528.3µs</code></td>
  <td><code>559.3µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>19.786µs</code></td>
  <td><code>975.153µs</code></td>
  <td><b>49.3x</b></td>
  <td><code>193.032µs</code></td>
  <td><code>172.331µs</code></td>
  <td><b>0.9x</b></td>
  <td><code>999.875µs</code></td>
  <td><code>2.328875ms</code></td>
  <td><b>2.3x</b></td>
  <td><code>532µs</code></td>
  <td><code>593µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>570.8µs</code></td>
  <td><code>586.1µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>89.772µs</code></td>
  <td><code>998.417µs</code></td>
  <td>—</td>
  <td><code>450.17µs</code></td>
  <td><code>998.858µs</code></td>
  <td>—</td>
  <td><code>13.443084ms</code></td>
  <td><code>17.302209ms</code></td>
  <td>—</td>
  <td><code>2.495ms</code></td>
  <td><code>1.038ms</code></td>
  <td>—</td>
  <td><code>999.5µs</code></td>
  <td><code>703.3µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>1.037067ms</code></td>
  <td><code>127.86357ms</code></td>
  <td>—</td>
  <td><code>76.449µs</code></td>
  <td><code>124.470808ms</code></td>
  <td>—</td>
  <td><code>69.979916ms</code></td>
  <td><code>139.213875ms</code></td>
  <td>—</td>
  <td><code>2.421ms</code></td>
  <td><code>2.114ms</code></td>
  <td>—</td>
  <td><code>410.9µs</code></td>
  <td><code>1.8539ms</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>74.852µs</code></td>
  <td><code>71.864µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>66.364µs</code></td>
  <td><code>66.666µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>78.757µs</code></td>
  <td><code>651.656µs</code></td>
  <td><b>8.3x</b></td>
  <td><code>257.126µs</code></td>
  <td><code>493.406µs</code></td>
  <td><b>1.9x</b></td>
  <td><code>545.179µs</code></td>
  <td><code>530.222µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>107.872µs</code></td>
  <td><code>169.075µs</code></td>
  <td><b>1.6x</b></td>
  <td><code>97.277µs</code></td>
  <td><code>156.462µs</code></td>
  <td><b>1.6x</b></td>
  <td><code>125.558µs</code></td>
  <td><code>1.52832ms</code></td>
  <td><b>12.2x</b></td>
  <td><code>832.944µs</code></td>
  <td><code>1.954466ms</code></td>
  <td><b>2.3x</b></td>
  <td><code>344.749µs</code></td>
  <td><code>274.237µs</code></td>
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