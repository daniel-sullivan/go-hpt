<p align="center">
  <img src="https://img.shields.io/badge/linux_tests-passing-brightgreen" alt="linux: tests">
  <img src="https://img.shields.io/badge/linux_coverage-84.6%25-brightgreen" alt="linux: coverage">
  <img src="https://img.shields.io/badge/macOS_tests-passing-brightgreen" alt="macos: tests">
  <img src="https://img.shields.io/badge/macOS_coverage-84.6%25-brightgreen" alt="macos: coverage">
  <img src="https://img.shields.io/badge/windows_tests-passing-brightgreen" alt="windows: tests">
  <img src="https://img.shields.io/badge/windows_coverage-85.6%25-brightgreen" alt="windows: coverage">
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
  <td><code>66.238µs</code></td>
  <td><code>973.263µs</code></td>
  <td><b>14.7x</b></td>
  <td><code>67.804µs</code></td>
  <td><code>974.477µs</code></td>
  <td><b>14.4x</b></td>
  <td><code>305ns</code></td>
  <td><code>66.998µs</code></td>
  <td><b>219.7x</b></td>
  <td><code>1.443µs</code></td>
  <td><code>60.407µs</code></td>
  <td><b>41.9x</b></td>
  <td><code>445.504µs</code></td>
  <td><code>444.587µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>500µs</b></td>
  <td><code>71.85µs</code></td>
  <td><code>572.548µs</code></td>
  <td><b>8.0x</b></td>
  <td><code>70.547µs</code></td>
  <td><code>572.383µs</code></td>
  <td><b>8.1x</b></td>
  <td><code>32.259µs</code></td>
  <td><code>199.704µs</code></td>
  <td><b>6.2x</b></td>
  <td><code>115.179µs</code></td>
  <td><code>271.279µs</code></td>
  <td><b>2.4x</b></td>
  <td><code>246.904µs</code></td>
  <td><code>539.32µs</code></td>
  <td><b>2.2x</b></td>
</tr>
<tr>
  <td><b>1ms</b></td>
  <td><code>70.369µs</code></td>
  <td><code>74.187µs</code></td>
  <td><b>1.1x</b></td>
  <td><code>70.406µs</code></td>
  <td><code>72.285µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>68.479µs</code></td>
  <td><code>401.781µs</code></td>
  <td><b>5.9x</b></td>
  <td><code>284.838µs</code></td>
  <td><code>523.412µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>341.775µs</code></td>
  <td><code>519.309µs</code></td>
  <td><b>1.5x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>105.449µs</code></td>
  <td><code>186.597µs</code></td>
  <td><b>1.8x</b></td>
  <td><code>94.392µs</code></td>
  <td><code>192.036µs</code></td>
  <td><b>2.0x</b></td>
  <td><code>240.365µs</code></td>
  <td><code>1.565406ms</code></td>
  <td><b>6.5x</b></td>
  <td><code>944.956µs</code></td>
  <td><code>1.73145ms</code></td>
  <td><b>1.8x</b></td>
  <td><code>251.268µs</code></td>
  <td><code>247.79µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <th rowspan="6" align="left">Ticker</th>
  <td><b>Median jitter</b></td>
  <td><code>5.084µs</code></td>
  <td><code>90.717µs</code></td>
  <td>—</td>
  <td><code>5.135µs</code></td>
  <td><code>77.55µs</code></td>
  <td>—</td>
  <td><code>9.833µs</code></td>
  <td><code>10.875µs</code></td>
  <td>—</td>
  <td><code>34µs</code></td>
  <td><code>82µs</code></td>
  <td>—</td>
  <td><code>52.5µs</code></td>
  <td><code>95.5µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Mean jitter</b></td>
  <td><code>7.752µs</code></td>
  <td><code>106.992µs</code></td>
  <td><b>13.8x</b></td>
  <td><code>5.201µs</code></td>
  <td><code>92.803µs</code></td>
  <td><b>17.8x</b></td>
  <td><code>183.485µs</code></td>
  <td><code>67.492µs</code></td>
  <td><b>0.4x</b></td>
  <td><code>185.414µs</code></td>
  <td><code>141.22µs</code></td>
  <td><b>0.8x</b></td>
  <td><code>155.481µs</code></td>
  <td><code>194.251µs</code></td>
  <td><b>1.2x</b></td>
</tr>
<tr>
  <td><b>p95 jitter</b></td>
  <td><code>16.194µs</code></td>
  <td><code>124.319µs</code></td>
  <td>—</td>
  <td><code>10.486µs</code></td>
  <td><code>88.128µs</code></td>
  <td>—</td>
  <td><code>820.667µs</code></td>
  <td><code>285.125µs</code></td>
  <td>—</td>
  <td><code>460µs</code></td>
  <td><code>372µs</code></td>
  <td>—</td>
  <td><code>552.2µs</code></td>
  <td><code>566.7µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>p99 jitter</b></td>
  <td><code>63.492µs</code></td>
  <td><code>981.32µs</code></td>
  <td><b>15.5x</b></td>
  <td><code>19.482µs</code></td>
  <td><code>985.6µs</code></td>
  <td><b>50.6x</b></td>
  <td><code>1.871166ms</code></td>
  <td><code>1.038083ms</code></td>
  <td><b>0.6x</b></td>
  <td><code>1.286ms</code></td>
  <td><code>604µs</code></td>
  <td><b>0.5x</b></td>
  <td><code>578.5µs</code></td>
  <td><code>586.2µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>Max jitter</b></td>
  <td><code>434.452µs</code></td>
  <td><code>999.319µs</code></td>
  <td>—</td>
  <td><code>117.964µs</code></td>
  <td><code>998.978µs</code></td>
  <td>—</td>
  <td><code>44.205833ms</code></td>
  <td><code>5.704416ms</code></td>
  <td>—</td>
  <td><code>21.144ms</code></td>
  <td><code>12.426ms</code></td>
  <td>—</td>
  <td><code>999.6µs</code></td>
  <td><code>602.3µs</code></td>
  <td>—</td>
</tr>
<tr>
  <td><b>Total drift</b></td>
  <td><code>1.043731ms</code></td>
  <td><code>132.700811ms</code></td>
  <td>—</td>
  <td><code>96.154µs</code></td>
  <td><code>118.437425ms</code></td>
  <td>—</td>
  <td><code>140.878333ms</code></td>
  <td><code>41.17825ms</code></td>
  <td>—</td>
  <td><code>106.342ms</code></td>
  <td><code>42.441ms</code></td>
  <td>—</td>
  <td><code>431µs</code></td>
  <td><code>1.339ms</code></td>
  <td>—</td>
</tr>
<tr>
  <th rowspan="2" align="left">Timer</th>
  <td><b>1ms</b></td>
  <td><code>74.723µs</code></td>
  <td><code>75.147µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>75.52µs</code></td>
  <td><code>71.982µs</code></td>
  <td><b>1.0x</b></td>
  <td><code>107.865µs</code></td>
  <td><code>396.424µs</code></td>
  <td><b>3.7x</b></td>
  <td><code>239.165µs</code></td>
  <td><code>465.453µs</code></td>
  <td><b>1.9x</b></td>
  <td><code>530.938µs</code></td>
  <td><code>518.021µs</code></td>
  <td><b>1.0x</b></td>
</tr>
<tr>
  <td><b>5ms</b></td>
  <td><code>94.936µs</code></td>
  <td><code>161.665µs</code></td>
  <td><b>1.7x</b></td>
  <td><code>98.088µs</code></td>
  <td><code>157.711µs</code></td>
  <td><b>1.6x</b></td>
  <td><code>308.546µs</code></td>
  <td><code>1.494826ms</code></td>
  <td><b>4.8x</b></td>
  <td><code>1.106487ms</code></td>
  <td><code>1.66889ms</code></td>
  <td><b>1.5x</b></td>
  <td><code>368.21µs</code></td>
  <td><code>271.301µs</code></td>
  <td><b>0.7x</b></td>
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