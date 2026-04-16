// Command bench runs hpt benchmarks comparing hpt vs the standard time package.
//
// Install and run:
//
//	go run github.com/daniel-sullivan/go-hpt/cmd/bench@latest
//
// Pass -json to emit machine-readable JSON (same schema used to generate the
// README benchmark table).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"time"

	hpt "github.com/daniel-sullivan/go-hpt"
)

// JSON report types — matches the schema consumed by update-readme.py.
type sleepComparison struct {
	Duration   string `json:"duration"`
	HPTMean    string `json:"hpt_mean"`
	HPTP99     string `json:"hpt_p99"`
	StdlibMean string `json:"stdlib_mean"`
	StdlibP99  string `json:"stdlib_p99"`
	MeanImpr   string `json:"mean_improvement"`
	P99Impr    string `json:"p99_improvement"`
}

type tickerComparison struct {
	HPTMedianJitter    string `json:"hpt_median_jitter"`
	HPTMeanJitter      string `json:"hpt_mean_jitter"`
	HPTP95Jitter       string `json:"hpt_p95_jitter"`
	HPTP99Jitter       string `json:"hpt_p99_jitter"`
	HPTMaxJitter       string `json:"hpt_max_jitter"`
	HPTTotalDrift      string `json:"hpt_total_drift"`
	StdlibMedianJitter string `json:"stdlib_median_jitter"`
	StdlibMeanJitter   string `json:"stdlib_mean_jitter"`
	StdlibP95Jitter    string `json:"stdlib_p95_jitter"`
	StdlibP99Jitter    string `json:"stdlib_p99_jitter"`
	StdlibMaxJitter    string `json:"stdlib_max_jitter"`
	StdlibTotalDrift   string `json:"stdlib_total_drift"`
	MeanImpr           string `json:"mean_improvement"`
	P99Impr            string `json:"p99_improvement"`
}

type timerComparison struct {
	Duration   string `json:"duration"`
	HPTMean    string `json:"hpt_mean"`
	HPTP99     string `json:"hpt_p99"`
	StdlibMean string `json:"stdlib_mean"`
	StdlibP99  string `json:"stdlib_p99"`
	MeanImpr   string `json:"mean_improvement"`
	P99Impr    string `json:"p99_improvement"`
}

type benchmarkReport struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Cgo      bool   `json:"cgo"`

	Sleep  []sleepComparison `json:"sleep"`
	Ticker tickerComparison  `json:"ticker"`
	Timer  []timerComparison `json:"timer"`
}

func main() {
	jsonOut := flag.String("json", "", "write JSON report to `file` (- for stdout)")
	flag.Parse()

	report := benchmarkReport{
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		Cgo:      cgoEnabled(),
	}

	fmt.Printf("hpt benchmark — %s/%s (cgo=%v)\n\n", report.Platform, report.Arch, report.Cgo)

	// --- Sleep ---
	sleepDurations := []time.Duration{
		100 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		5 * time.Millisecond,
	}

	fmt.Println("Sleep (overshoot, lower is better)")
	fmt.Printf("  %-10s  %-14s %-14s %-14s %-14s  %s\n",
		"duration", "hpt mean", "hpt p99", "time mean", "time p99", "improvement")
	for _, d := range sleepDurations {
		hptS := measureSleep(1000, d, func(d time.Duration) { hpt.Sleep(d) })
		stdS := measureSleep(1000, d, func(d time.Duration) { time.Sleep(d) })

		meanImpr := safeDivide(stdS.mean, hptS.mean)
		p99Impr := safeDivide(stdS.p99, hptS.p99)

		fmt.Printf("  %-10s  %-14s %-14s %-14s %-14s  %.1fx mean, %.1fx p99\n",
			d, hptS.mean, hptS.p99, stdS.mean, stdS.p99, meanImpr, p99Impr)

		report.Sleep = append(report.Sleep, sleepComparison{
			Duration:   d.String(),
			HPTMean:    hptS.mean.String(),
			HPTP99:     hptS.p99.String(),
			StdlibMean: stdS.mean.String(),
			StdlibP99:  stdS.p99.String(),
			MeanImpr:   fmt.Sprintf("%.1fx", meanImpr),
			P99Impr:    fmt.Sprintf("%.1fx", p99Impr),
		})
	}

	// --- Ticker ---
	fmt.Println("\nTicker (1ms period, 2000 ticks)")
	tickerPeriod := 1 * time.Millisecond
	tickerCount := 2000

	hptTk := measureTicker(tickerCount, tickerPeriod, func(period time.Duration) (<-chan time.Time, func()) {
		tk := hpt.NewTicker(period)
		return tk.C, tk.Stop
	})
	stdTk := measureTicker(tickerCount, tickerPeriod, func(period time.Duration) (<-chan time.Time, func()) {
		tk := time.NewTicker(period)
		return tk.C, tk.Stop
	})

	meanImpr := safeDivide(stdTk.meanJitter, hptTk.meanJitter)
	p99Impr := safeDivide(stdTk.p99Jitter, hptTk.p99Jitter)

	fmt.Printf("  %-16s  %-14s  %s\n", "", "hpt", "time")
	fmt.Printf("  %-16s  %-14s  %s\n", "median jitter", hptTk.medianJitter, stdTk.medianJitter)
	fmt.Printf("  %-16s  %-14s  %s\n", "mean jitter", hptTk.meanJitter, stdTk.meanJitter)
	fmt.Printf("  %-16s  %-14s  %s\n", "p95 jitter", hptTk.p95Jitter, stdTk.p95Jitter)
	fmt.Printf("  %-16s  %-14s  %s\n", "p99 jitter", hptTk.p99Jitter, stdTk.p99Jitter)
	fmt.Printf("  %-16s  %-14s  %s\n", "max jitter", hptTk.maxJitter, stdTk.maxJitter)
	fmt.Printf("  %-16s  %-14s  %s\n", "total drift", hptTk.totalDrift, stdTk.totalDrift)
	fmt.Printf("  improvement: %.1fx mean, %.1fx p99\n", meanImpr, p99Impr)

	report.Ticker = tickerComparison{
		HPTMedianJitter:    hptTk.medianJitter.String(),
		HPTMeanJitter:      hptTk.meanJitter.String(),
		HPTP95Jitter:       hptTk.p95Jitter.String(),
		HPTP99Jitter:       hptTk.p99Jitter.String(),
		HPTMaxJitter:       hptTk.maxJitter.String(),
		HPTTotalDrift:      hptTk.totalDrift.String(),
		StdlibMedianJitter: stdTk.medianJitter.String(),
		StdlibMeanJitter:   stdTk.meanJitter.String(),
		StdlibP95Jitter:    stdTk.p95Jitter.String(),
		StdlibP99Jitter:    stdTk.p99Jitter.String(),
		StdlibMaxJitter:    stdTk.maxJitter.String(),
		StdlibTotalDrift:   stdTk.totalDrift.String(),
		MeanImpr:           fmt.Sprintf("%.1fx", meanImpr),
		P99Impr:            fmt.Sprintf("%.1fx", p99Impr),
	}

	// --- Timer ---
	timerDurations := []time.Duration{1 * time.Millisecond, 5 * time.Millisecond}

	fmt.Println("\nTimer (overshoot, lower is better)")
	fmt.Printf("  %-10s  %-14s %-14s %-14s %-14s  %s\n",
		"duration", "hpt mean", "hpt p99", "time mean", "time p99", "improvement")
	for _, d := range timerDurations {
		hptS := measureTimer(500, d, func(d time.Duration) {
			timer := hpt.NewTimer(d)
			<-timer.C
		})
		stdS := measureTimer(500, d, func(d time.Duration) {
			timer := time.NewTimer(d)
			<-timer.C
		})

		tmeanImpr := safeDivide(stdS.mean, hptS.mean)
		tp99Impr := safeDivide(stdS.p99, hptS.p99)

		fmt.Printf("  %-10s  %-14s %-14s %-14s %-14s  %.1fx mean, %.1fx p99\n",
			d, hptS.mean, hptS.p99, stdS.mean, stdS.p99, tmeanImpr, tp99Impr)

		report.Timer = append(report.Timer, timerComparison{
			Duration:   d.String(),
			HPTMean:    hptS.mean.String(),
			HPTP99:     hptS.p99.String(),
			StdlibMean: stdS.mean.String(),
			StdlibP99:  stdS.p99.String(),
			MeanImpr:   fmt.Sprintf("%.1fx", tmeanImpr),
			P99Impr:    fmt.Sprintf("%.1fx", tp99Impr),
		})
	}

	// --- JSON output ---
	if *jsonOut != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
			os.Exit(1)
		}
		if *jsonOut == "-" {
			fmt.Println("\n" + string(data))
		} else {
			if err := os.WriteFile(*jsonOut, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", *jsonOut, err)
				os.Exit(1)
			}
			fmt.Printf("\nJSON report written to %s\n", *jsonOut)
		}
	}
}

// --- measurement helpers ---

type stats struct {
	mean   time.Duration
	median time.Duration
	p99    time.Duration
	max    time.Duration
}

func measureSleep(iterations int, d time.Duration, sleepFn func(time.Duration)) stats {
	overshoots := make([]time.Duration, iterations)
	for i := range iterations {
		start := hpt.Now()
		sleepFn(d)
		elapsed := hpt.Since(start)
		overshoots[i] = elapsed - d
	}
	return computeStats(overshoots)
}

func measureTimer(iterations int, d time.Duration, fn func(time.Duration)) stats {
	overshoots := make([]time.Duration, iterations)
	for i := range iterations {
		start := hpt.Now()
		fn(d)
		elapsed := hpt.Since(start)
		overshoots[i] = elapsed - d
	}
	return computeStats(overshoots)
}

func computeStats(samples []time.Duration) stats {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	var sum float64
	for _, s := range samples {
		sum += float64(s)
	}
	n := len(samples)
	return stats{
		mean:   time.Duration(sum / float64(n)),
		median: samples[n/2],
		p99:    samples[int(float64(n)*0.99)],
		max:    samples[n-1],
	}
}

type tickerStats struct {
	totalDrift   time.Duration
	medianJitter time.Duration
	meanJitter   time.Duration
	p95Jitter    time.Duration
	p99Jitter    time.Duration
	maxJitter    time.Duration
}

func measureTicker(count int, period time.Duration, makeTicker func(time.Duration) (<-chan time.Time, func())) tickerStats {
	ch, stop := makeTicker(period)

	start := hpt.Now()
	jitters := make([]time.Duration, 0, count)
	var prevNano int64
	for i := range count {
		<-ch
		now := hpt.Now()
		if i > 0 {
			interval := time.Duration(now - prevNano)
			jitter := interval - period
			if jitter < 0 {
				jitter = -jitter
			}
			jitters = append(jitters, jitter)
		}
		prevNano = now
	}
	// Use the last tick's timestamp for drift, not post-stop time.
	// stop() may block (e.g. pthread_join) for up to one period.
	endNano := prevNano
	stop()
	totalDrift := time.Duration(endNano-start) - time.Duration(count)*period

	sort.Slice(jitters, func(i, j int) bool { return jitters[i] < jitters[j] })
	var sum float64
	for _, j := range jitters {
		sum += float64(j)
	}
	n := len(jitters)

	return tickerStats{
		totalDrift:   totalDrift,
		medianJitter: jitters[n/2],
		meanJitter:   time.Duration(sum / float64(n)),
		p95Jitter:    jitters[int(float64(n)*0.95)],
		p99Jitter:    jitters[int(float64(n)*0.99)],
		maxJitter:    jitters[n-1],
	}
}

func safeDivide(a, b time.Duration) float64 {
	if b == 0 {
		return math.Inf(1)
	}
	return float64(a) / float64(b)
}
