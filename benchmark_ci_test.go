package hpt

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"
)

type ciSleepComparison struct {
	Duration   string `json:"duration"`
	HPTMean    string `json:"hpt_mean"`
	HPTP99     string `json:"hpt_p99"`
	StdlibMean string `json:"stdlib_mean"`
	StdlibP99  string `json:"stdlib_p99"`
	MeanImpr   string `json:"mean_improvement"`
	P99Impr    string `json:"p99_improvement"`
}

type ciTickerComparison struct {
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

type ciTimerComparison struct {
	Duration   string `json:"duration"`
	HPTMean    string `json:"hpt_mean"`
	HPTP99     string `json:"hpt_p99"`
	StdlibMean string `json:"stdlib_mean"`
	StdlibP99  string `json:"stdlib_p99"`
	MeanImpr   string `json:"mean_improvement"`
	P99Impr    string `json:"p99_improvement"`
}

type ciBenchmarkReport struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Cgo      bool   `json:"cgo"`

	Sleep  []ciSleepComparison `json:"sleep"`
	Ticker ciTickerComparison  `json:"ticker"`
	Timer  []ciTimerComparison `json:"timer"`
}

// TestCIBenchmarks produces a JSON report for CI consumption.
// Gated by BENCH_OUTPUT env var (path to write JSON).
//
//	BENCH_OUTPUT=results.json go test -run TestCIBenchmarks -v -count=1 -timeout=5m
func TestCIBenchmarks(t *testing.T) {
	outPath := os.Getenv("BENCH_OUTPUT")
	if outPath == "" {
		t.Skip("set BENCH_OUTPUT=path/to/results.json to run")
	}

	report := ciBenchmarkReport{
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		Cgo:      cgoActive,
	}

	// --- Sleep: hpt vs time.Sleep ---
	sleepDurations := []time.Duration{
		100 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		5 * time.Millisecond,
	}
	for _, d := range sleepDurations {
		hptStats := measureSleepOvershoot(200, d, func(d time.Duration) { Sleep(d) })
		stdStats := measureSleepOvershoot(200, d, func(d time.Duration) { time.Sleep(d) })

		report.Sleep = append(report.Sleep, ciSleepComparison{
			Duration:   d.String(),
			HPTMean:    hptStats.mean.String(),
			HPTP99:     hptStats.p99.String(),
			StdlibMean: stdStats.mean.String(),
			StdlibP99:  stdStats.p99.String(),
			MeanImpr:   fmt.Sprintf("%.1fx", safeDivide(stdStats.mean, hptStats.mean)),
			P99Impr:    fmt.Sprintf("%.1fx", safeDivide(stdStats.p99, hptStats.p99)),
		})
	}

	// --- Ticker: hpt vs time.Ticker ---
	tickerPeriod := 1 * time.Millisecond
	tickerCount := 500

	hptTk := measureTickerJitterFull(tickerCount, tickerPeriod, func(period time.Duration) (<-chan time.Time, func()) {
		tk := NewTicker(period)
		return tk.C, tk.Stop
	})
	stdTk := measureTickerJitterFull(tickerCount, tickerPeriod, func(period time.Duration) (<-chan time.Time, func()) {
		tk := time.NewTicker(period)
		return tk.C, tk.Stop
	})

	report.Ticker = ciTickerComparison{
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
		MeanImpr:           fmt.Sprintf("%.1fx", safeDivide(stdTk.meanJitter, hptTk.meanJitter)),
		P99Impr:            fmt.Sprintf("%.1fx", safeDivide(stdTk.p99Jitter, hptTk.p99Jitter)),
	}

	// --- Timer: hpt vs time.Timer ---
	timerDurations := []time.Duration{1 * time.Millisecond, 5 * time.Millisecond}
	for _, d := range timerDurations {
		hptStats := measureTimerOvershoot(100, d, func(d time.Duration) {
			timer := NewTimer(d)
			<-timer.C
		})
		stdStats := measureTimerOvershoot(100, d, func(d time.Duration) {
			timer := time.NewTimer(d)
			<-timer.C
		})

		report.Timer = append(report.Timer, ciTimerComparison{
			Duration:   d.String(),
			HPTMean:    hptStats.mean.String(),
			HPTP99:     hptStats.p99.String(),
			StdlibMean: stdStats.mean.String(),
			StdlibP99:  stdStats.p99.String(),
			MeanImpr:   fmt.Sprintf("%.1fx", safeDivide(stdStats.mean, hptStats.mean)),
			P99Impr:    fmt.Sprintf("%.1fx", safeDivide(stdStats.p99, hptStats.p99)),
		})
	}

	// --- Write JSON ---
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		t.Fatalf("write %s: %v", outPath, err)
	}
	t.Logf("wrote benchmark report to %s (cgo=%v)", outPath, cgoActive)
}

type tickerStatsFull struct {
	totalDrift   time.Duration
	medianJitter time.Duration
	meanJitter   time.Duration
	p95Jitter    time.Duration
	p99Jitter    time.Duration
	maxJitter    time.Duration
}

func measureTickerJitterFull(count int, period time.Duration, makeTicker func(time.Duration) (<-chan time.Time, func())) tickerStatsFull {
	ch, stop := makeTicker(period)

	start := monotonicNow()
	jitters := make([]time.Duration, 0, count)
	var prevNano int64
	for i := 0; i < count; i++ {
		<-ch
		now := monotonicNow()
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
	stop()
	totalDrift := time.Duration(monotonicNow()-start) - time.Duration(count)*period

	sort.Slice(jitters, func(i, j int) bool { return jitters[i] < jitters[j] })
	var sum float64
	for _, j := range jitters {
		sum += float64(j)
	}
	n := len(jitters)

	return tickerStatsFull{
		totalDrift:   totalDrift,
		medianJitter: jitters[n/2],
		meanJitter:   time.Duration(sum / float64(n)),
		p95Jitter:    jitters[int(float64(n)*0.95)],
		p99Jitter:    jitters[int(float64(n)*0.99)],
		maxJitter:    jitters[n-1],
	}
}

func measureTimerOvershoot(iterations int, d time.Duration, fn func(time.Duration)) overshootStats {
	overshoots := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		fn(d)
		elapsed := time.Since(start)
		overshoots[i] = elapsed - d
	}
	sort.Slice(overshoots, func(i, j int) bool { return overshoots[i] < overshoots[j] })

	var sum float64
	for _, o := range overshoots {
		sum += float64(o)
	}
	return overshootStats{
		mean:   time.Duration(sum / float64(iterations)),
		median: overshoots[iterations/2],
		p99:    overshoots[int(float64(iterations)*0.99)],
		max:    overshoots[iterations-1],
	}
}
