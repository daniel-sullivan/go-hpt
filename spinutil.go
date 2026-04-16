package hpt

import (
	"github.com/daniel-sullivan/go-hpt/internal/counter"
)

var (
	counterReady      bool
	counterTicksPerNs float64
)

func init() {
	if !counter.Supported() {
		return
	}

	// Calibrate: spin for ~1ms three times, keep the tightest bracket.
	var bestDtNs int64
	var bestDtCnt uint64
	for i := 0; i < 3; i++ {
		ns1 := monotonicNow()
		c1 := counter.Read()
		target := ns1 + 1_000_000 // 1ms
		for monotonicNow() < target {
		}
		c2 := counter.Read()
		ns2 := monotonicNow()

		dtNs := ns2 - ns1
		dtCnt := c2 - c1
		if dtCnt == 0 || dtNs == 0 {
			continue
		}
		if bestDtNs == 0 || dtNs < bestDtNs {
			bestDtNs = dtNs
			bestDtCnt = dtCnt
		}
	}

	if bestDtCnt == 0 || bestDtNs == 0 {
		return
	}

	ticksPerNs := float64(bestDtCnt) / float64(bestDtNs)

	// Sanity: counter frequency should be between 1 MHz and 10 GHz.
	freqHz := ticksPerNs * 1e9
	if freqHz < 1e6 || freqHz > 10e9 {
		return
	}

	counterTicksPerNs = ticksPerNs
	counterReady = true
}

// spinUntil busy-waits until the monotonic clock reaches deadline.
// When a CPU counter is available it reads the counter directly (~5-10 ns
// per iteration) instead of going through the OS clock API (~25-50 ns).
func spinUntil(deadline int64) {
	if !counterReady {
		for monotonicNow() < deadline {
		}
		return
	}

	// Snapshot both clocks to convert the nanosecond deadline into counter
	// ticks. Using a fresh reference each call keeps conversion error small
	// regardless of how long ago calibration ran.
	refNs := monotonicNow()
	refCnt := counter.Read()
	deltaNs := deadline - refNs
	if deltaNs <= 0 {
		return
	}
	deadlineCnt := refCnt + uint64(float64(deltaNs)*counterTicksPerNs)
	for counter.Read() < deadlineCnt {
	}
}
