package hpt

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/daniel-sullivan/go-hpt/internal/counter"
)

func BenchmarkMonotonicNow(b *testing.B) {
	for b.Loop() {
		monotonicNow()
	}
}

func BenchmarkCounterRead(b *testing.B) {
	if !counterReady {
		b.Skip("CPU counter not available")
	}

	for b.Loop() {
		counter.Read()
	}
}

func BenchmarkSpinUntil1us(b *testing.B) {
	for b.Loop() {
		spinUntil(monotonicNow() + 1000)
	}
}

func BenchmarkSleep1ms(b *testing.B) {
	d := 1 * time.Millisecond
	for b.Loop() {
		Sleep(d)
	}
}

func BenchmarkSleep100us(b *testing.B) {
	d := 100 * time.Microsecond
	for b.Loop() {
		Sleep(d)
	}
}

func BenchmarkSleep10ms(b *testing.B) {
	d := 10 * time.Millisecond
	for b.Loop() {
		Sleep(d)
	}
}

func BenchmarkTickerJitter(b *testing.B) {
	period := 1 * time.Millisecond
	ticker := NewTicker(period)
	defer ticker.Stop()

	b.ResetTimer()
	var prev time.Time
	first := true
	for b.Loop() {
		tick := <-ticker.C
		if !first {
			_ = tick.Sub(prev) - period // jitter
		}
		prev = tick
		first = false
	}
}

// TestPrecisionReport prints a statistical summary of sleep precision.
// Run with: go test -run TestPrecisionReport -v
func TestPrecisionReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping precision report in short mode")
	}

	durations := []time.Duration{
		100 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		5 * time.Millisecond,
	}

	for _, d := range durations {
		t.Run(d.String(), func(t *testing.T) {
			const iterations = 200
			overshoots := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				start := time.Now()
				Sleep(d)
				elapsed := time.Since(start)
				overshoots[i] = elapsed - d
			}

			sort.Slice(overshoots, func(i, j int) bool {
				return overshoots[i] < overshoots[j]
			})

			var sum float64
			for _, o := range overshoots {
				sum += float64(o)
			}
			mean := time.Duration(sum / float64(iterations))

			median := overshoots[iterations/2]
			p95 := overshoots[int(float64(iterations)*0.95)]
			p99 := overshoots[int(float64(iterations)*0.99)]
			maxO := overshoots[iterations-1]
			minO := overshoots[0]

			t.Logf("hpt.Sleep(%v) precision over %d iterations:", d, iterations)
			t.Logf("  min overshoot:  %v", minO)
			t.Logf("  mean overshoot: %v", mean)
			t.Logf("  median:         %v", median)
			t.Logf("  p95:            %v", p95)
			t.Logf("  p99:            %v", p99)
			t.Logf("  max overshoot:  %v", maxO)
		})
	}
}

// TestStdlibComparison compares hpt.Sleep vs time.Sleep precision.
// Run with: go test -run TestStdlibComparison -v
func TestStdlibComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stdlib comparison in short mode")
	}

	durations := []time.Duration{
		1 * time.Millisecond,
		5 * time.Millisecond,
	}

	for _, d := range durations {
		t.Run(d.String(), func(t *testing.T) {
			const iterations = 100

			// Measure hpt.Sleep
			hptOvershoot := measureSleepOvershoot(iterations, d, func(d time.Duration) {
				Sleep(d)
			})

			// Measure time.Sleep
			stdOvershoot := measureSleepOvershoot(iterations, d, func(d time.Duration) {
				time.Sleep(d)
			})

			t.Logf("Sleep(%v) — %d iterations:", d, iterations)
			t.Logf("  hpt:    mean=%v  median=%v  p99=%v  max=%v",
				hptOvershoot.mean, hptOvershoot.median, hptOvershoot.p99, hptOvershoot.max)
			t.Logf("  stdlib: mean=%v  median=%v  p99=%v  max=%v",
				stdOvershoot.mean, stdOvershoot.median, stdOvershoot.p99, stdOvershoot.max)
			t.Logf("  improvement: %.1fx mean, %.1fx p99",
				safeDivide(stdOvershoot.mean, hptOvershoot.mean),
				safeDivide(stdOvershoot.p99, hptOvershoot.p99))
		})
	}
}

// TestTickerDriftReport prints a statistical summary of ticker drift.
// Run with: go test -run TestTickerDriftReport -v
func TestTickerDriftReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ticker drift report in short mode")
	}

	period := 1 * time.Millisecond
	count := 500
	ticker := NewTicker(period)
	defer ticker.Stop()

	start := time.Now()
	jitters := make([]time.Duration, 0, count)
	var prev time.Time

	for i := 0; i < count; i++ {
		tick := <-ticker.C
		if i > 0 {
			interval := tick.Sub(prev)
			jitter := interval - period
			if jitter < 0 {
				jitter = -jitter
			}
			jitters = append(jitters, jitter)
		}
		prev = tick
	}

	elapsed := time.Since(start)
	expected := time.Duration(count) * period
	totalDrift := elapsed - expected

	sort.Slice(jitters, func(i, j int) bool { return jitters[i] < jitters[j] })

	var sum float64
	for _, j := range jitters {
		sum += float64(j)
	}
	mean := time.Duration(sum / float64(len(jitters)))

	t.Logf("Ticker(%v) — %d ticks:", period, count)
	t.Logf("  total drift:  %v (over %v expected)", totalDrift, expected)
	t.Logf("  mean jitter:  %v", mean)
	t.Logf("  median:       %v", jitters[len(jitters)/2])
	t.Logf("  p95:          %v", jitters[int(float64(len(jitters))*0.95)])
	t.Logf("  p99:          %v", jitters[int(float64(len(jitters))*0.99)])
	t.Logf("  max jitter:   %v", jitters[len(jitters)-1])
}

type overshootStats struct {
	mean   time.Duration
	median time.Duration
	p99    time.Duration
	max    time.Duration
}

func measureSleepOvershoot(iterations int, d time.Duration, sleepFn func(time.Duration)) overshootStats {
	overshoots := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		sleepFn(d)
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

func safeDivide(a, b time.Duration) float64 {
	if b == 0 {
		return math.Inf(1)
	}
	return float64(a) / float64(b)
}
