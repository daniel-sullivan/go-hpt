<p align="center">
  <img src="https://img.shields.io/badge/linux_tests-passing-brightgreen" alt="linux: tests">
  <img src="https://img.shields.io/badge/linux_coverage-82.7%25-brightgreen" alt="linux: coverage">
  <img src="https://img.shields.io/badge/macOS_tests-passing-brightgreen" alt="macos: tests">
  <img src="https://img.shields.io/badge/macOS_coverage-82.7%25-brightgreen" alt="macos: coverage">
  <img src="https://img.shields.io/badge/windows_tests-passing-brightgreen" alt="windows: tests">
  <img src="https://img.shields.io/badge/windows_coverage-87.1%25-brightgreen" alt="windows: coverage">
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
  <td><code>63.178µs</code></td>
  <td><code>971.598µs</code></td>
  <td><b>15.4x</b></td>
  <td><code>65.06µs</code></td>
  <td><code>972.779µs</code></td>
  <td><b>15.0x</b></td>
  <td><code>121ns</code></td>
  <td><code>106.287µs</code></td>
  <td><b>878.4x</b></td>
  <td><code>18.039µs</code></td>
  <td><code>102.638µs</code></td>
  <td><b>5.7x</b></td>
  <td><code>488.68µs</code></td>
  <td><code>463.418µs</code></td>
  <td><b>0.9x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>68.463µs</code></td>
  <td><code>571.198µs</code></td>
  <td><b>8.3x</b></td>
  <td><code>68.617µs</code></td>
  <td><code>574.406µs</code></td>
  <td><b>8.4x</b></td>
  <td><code>176.638µs</code></td>
  <td><code>189.426µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>117.197µs</code></td>
  <td><code>118.791µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>344.544µs</code></td>
  <td><code>564.085µs</code></td>
  <td><b>1.6x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>69.77µs</code></td>
  <td><code>72.326µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>72.568µs</code></td>
  <td><code>77.479µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>42.545µs</code></td>
  <td><code>650.745µs</code></td>
  <td><b>15.3x</b></td>
  <td><code>62.51µs</code></td>
  <td><code>197.385µs</code></td>
  <td><b>3.2x</b></td>
  <td><code>361.281µs</code></td>
  <td><code>528.03µs</code></td>
  <td><b>1.5x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>86.653µs</code></td>
  <td><code>152.492µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>104.27µs</code></td>
  <td><code>154.532µs</code></td>
  <td><b>1.5x</b></td>
  <td><code>290.695µs</code></td>
  <td><code>1.502063ms</code></td>
  <td><b>5.2x</b></td>
  <td><code>144.71µs</code></td>
  <td><code>820.006µs</code></td>
  <td><b>5.7x</b></td>
  <td><code>301.579µs</code></td>
  <td><code>308.192µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>1.699µs</code></td>
  <td><code>88.363µs</code></td>
  <td>—</td>
  <td><code>6.01µs</code></td>
  <td><code>79.33µs</code></td>
  <td>—</td>
  <td><code>37.167µs</code></td>
  <td><code>67.584µs</code></td>
  <td>—</td>
  <td><code>30µs</code></td>
  <td><code>45µs</code></td>
  <td>—</td>
  <td><code>59.4µs</code></td>
  <td><code>107.7µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>3.278µs</code></td>
  <td><code>110.684µs</code></td>
  <td><b>33.8x</b></td>
  <td><code>6.944µs</code></td>
  <td><code>97.687µs</code></td>
  <td><b>14.1x</b></td>
  <td><code>200.759µs</code></td>
  <td><code>215.167µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>64.356µs</code></td>
  <td><code>111.607µs</code></td>
  <td><b>1.7x</b></td>
  <td><code>193.989µs</code></td>
  <td><code>211.377µs</code></td>
  <td><b>1.1x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>9.142µs</code></td>
  <td><code>124.871µs</code></td>
  <td>—</td>
  <td><code>18.159µs</code></td>
  <td><code>113.569µs</code></td>
  <td>—</td>
  <td><code>999.625µs</code></td>
  <td><code>981.333µs</code></td>
  <td>—</td>
  <td><code>321µs</code></td>
  <td><code>412µs</code></td>
  <td>—</td>
  <td><code>559.2µs</code></td>
  <td><code>573µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>45.274µs</code></td>
  <td><code>982.205µs</code></td>
  <td><b>21.7x</b></td>
  <td><code>43.185µs</code></td>
  <td><code>985.379µs</code></td>
  <td><b>22.8x</b></td>
  <td><code>1.853125ms</code></td>
  <td><code>2.190209ms</code></td>
  <td><b>1.2x</b></td>
  <td><code>458µs</code></td>
  <td><code>781µs</code></td>
  <td><b>1.7x</b></td>
  <td><code>999.7µs</code></td>
  <td><code>610.6µs</code></td>
  <td><b>0.6x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>60.353µs</code></td>
  <td><code>985.569µs</code></td>
  <td>—</td>
  <td><code>81.788µs</code></td>
  <td><code>998.647µs</code></td>
  <td>—</td>
  <td><code>3.530625ms</code></td>
  <td><code>3.139958ms</code></td>
  <td>—</td>
  <td><code>522µs</code></td>
  <td><code>3.397ms</code></td>
  <td>—</td>
  <td><code>5.6784ms</code></td>
  <td><code>3.9927ms</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>1.011005ms</code></td>
  <td><code>28.095077ms</code></td>
  <td>—</td>
  <td><code>93.68µs</code></td>
  <td><code>32.998274ms</code></td>
  <td>—</td>
  <td><code>10.494459ms</code></td>
  <td><code>38.216666ms</code></td>
  <td>—</td>
  <td><code>78µs</code></td>
  <td><code>5.529ms</code></td>
  <td>—</td>
  <td><code>7.4598ms</code></td>
  <td><code>5.4625ms</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>73.926µs</code></td>
  <td><code>70.883µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>78.902µs</code></td>
  <td><code>81.832µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>123.766µs</code></td>
  <td><code>634.507µs</code></td>
  <td><b>5.1x</b></td>
  <td><code>124.677µs</code></td>
  <td><code>338.007µs</code></td>
  <td><b>2.7x</b></td>
  <td><code>531.225µs</code></td>
  <td><code>518.13µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>113.168µs</code></td>
  <td><code>142.659µs</code></td>
  <td><b>1.3x</b></td>
  <td><code>126.335µs</code></td>
  <td><code>156.917µs</code></td>
  <td><b>1.2x</b></td>
  <td><code>550.209µs</code></td>
  <td><code>1.179151ms</code></td>
  <td><b>2.1x</b></td>
  <td><code>314.872µs</code></td>
  <td><code>1.717948ms</code></td>
  <td><b>5.5x</b></td>
  <td><code>379.049µs</code></td>
  <td><code>317.187µs</code></td>
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