//go:build darwin && cgo

package hpt

/*
#include <pthread.h>
#include <unistd.h>
#include <stdlib.h>
#include <stdatomic.h>
#include <sys/event.h>
#include <mach/mach_time.h>

static mach_timebase_info_data_t _tb;
static int _tb_init = 0;

static void hpt_ensure_timebase(void) {
	if (!_tb_init) {
		mach_timebase_info(&_tb);
		_tb_init = 1;
	}
}

static long long hpt_clock_now(void) {
	hpt_ensure_timebase();
	uint64_t mt = mach_absolute_time();
	return (long long)(mt * _tb.numer / _tb.denom);
}

// hpt_sleep_until_impl uses kevent(EVFILT_TIMER) with NOTE_CRITICAL to
// bypass macOS timer coalescing, plus a mach_absolute_time spin tail.
// Both mach_wait_until and nanosleep are subject to coalescing on macOS,
// but NOTE_CRITICAL explicitly opts out.
static void hpt_sleep_until_impl(long long deadline_ns) {
	hpt_ensure_timebase();
	uint64_t deadline_mach = (uint64_t)deadline_ns * _tb.denom / _tb.numer;

	for (;;) {
		uint64_t now_mach = mach_absolute_time();
		if (now_mach >= deadline_mach) return;

		uint64_t remaining_mach = deadline_mach - now_mach;
		long long remaining_ns = (long long)(remaining_mach * _tb.numer / _tb.denom);

		if (remaining_ns <= 100000) { // 100µs: spin
			while (mach_absolute_time() < deadline_mach) {}
			return;
		}

		// kevent with NOTE_NSECONDS | NOTE_CRITICAL for the bulk
		int kq = kqueue();
		if (kq < 0) {
			while (mach_absolute_time() < deadline_mach) {}
			return;
		}

		long long sleep_ns = remaining_ns - 50000; // leave 50µs for spin
		if (sleep_ns < 0) sleep_ns = remaining_ns / 2;

		struct kevent kev;
		EV_SET(&kev, 0, EVFILT_TIMER, EV_ADD | EV_ONESHOT | EV_ENABLE,
		       NOTE_NSECONDS | NOTE_CRITICAL, sleep_ns, NULL);
		struct kevent out;
		kevent(kq, &kev, 1, &out, 1, NULL);
		close(kq);

		// Spin for the tail
		while (mach_absolute_time() < deadline_mach) {}
		return;
	}
}

// --- pthread ticker ---

typedef struct {
	int         pipe_w;
	long long   start_ns;
	long long   period_ns;
	atomic_int  stop;
	pthread_t   thread;
} hpt_ticker_t;

static void* hpt_ticker_thread(void* arg) {
	hpt_ticker_t* t = (hpt_ticker_t*)arg;
	hpt_ensure_timebase();
	long long tick = 0;

	// Pre-create a kqueue for the ticker loop to avoid per-tick allocation.
	int kq = kqueue();

	while (!atomic_load(&t->stop)) {
		tick++;
		long long deadline_ns = t->start_ns + tick * t->period_ns;
		uint64_t deadline_mach = (uint64_t)deadline_ns * _tb.denom / _tb.numer;

		// Use kevent for bulk sleep if kqueue is available.
		uint64_t now_mach = mach_absolute_time();
		if (kq >= 0 && deadline_mach > now_mach) {
			long long remaining_ns = (long long)((deadline_mach - now_mach) * _tb.numer / _tb.denom);
			long long sleep_ns = remaining_ns - 50000;
			if (sleep_ns > 0) {
				struct kevent kev;
				EV_SET(&kev, 0, EVFILT_TIMER, EV_ADD | EV_ONESHOT | EV_ENABLE,
				       NOTE_NSECONDS | NOTE_CRITICAL, sleep_ns, NULL);
				struct kevent out;
				kevent(kq, &kev, 1, &out, 1, NULL);
			}
		}

		// Spin for the tail.
		while (mach_absolute_time() < deadline_mach) {}

		if (atomic_load(&t->stop)) break;
		char b = 1;
		write(t->pipe_w, &b, 1);
	}

	if (kq >= 0) close(kq);
	close(t->pipe_w);
	return NULL;
}

static hpt_ticker_t* hpt_ticker_start(long long period_ns, int pipe_w) {
	hpt_ticker_t* t = (hpt_ticker_t*)malloc(sizeof(hpt_ticker_t));
	t->pipe_w     = pipe_w;
	t->start_ns   = hpt_clock_now();
	t->period_ns  = period_ns;
	atomic_store(&t->stop, 0);

	pthread_attr_t attr;
	pthread_attr_init(&attr);
	pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
	pthread_create(&t->thread, &attr, hpt_ticker_thread, t);
	pthread_attr_destroy(&attr);
	return t;
}

static void hpt_ticker_stop(hpt_ticker_t* t) {
	atomic_store(&t->stop, 1);
	pthread_join(t->thread, NULL);
	free(t);
}

static int hpt_pipe(int fds[2]) {
	return pipe(fds);
}
*/
import "C"

import (
	"os"
	"time"
	"unsafe"
)

func monotonicNow() int64 {
	return int64(C.hpt_clock_now())
}

func sleepUntil(deadline int64) {
	C.hpt_sleep_until_impl(C.longlong(deadline))
}

func startTickerLoop(period time.Duration, c chan time.Time) (stop func()) {
	var fds [2]C.int
	if C.hpt_pipe((*C.int)(unsafe.Pointer(&fds[0]))) != 0 {
		return startTickerLoopFallback(period, c)
	}

	state := C.hpt_ticker_start(C.longlong(period.Nanoseconds()), fds[1])
	pipeR := os.NewFile(uintptr(fds[0]), "hpt-ticker")

	go func() {
		defer pipeR.Close()
		buf := make([]byte, 1)
		for {
			if _, err := pipeR.Read(buf); err != nil {
				return
			}
			select {
			case c <- time.Now():
			default:
			}
		}
	}()

	return func() {
		C.hpt_ticker_stop(state)
	}
}

func startTickerLoopFallback(period time.Duration, c chan time.Time) (stop func()) {
	done := make(chan struct{})
	go func() {
		start := monotonicNow()
		d := period.Nanoseconds()
		var tick int64
		for {
			tick++
			sleepUntil(start + tick*d)
			select {
			case <-done:
				return
			default:
			}
			select {
			case c <- time.Now():
			default:
			}
		}
	}()
	return func() { close(done) }
}
