//go:build linux && cgo

package hpt

/*
#include <pthread.h>
#include <unistd.h>
#include <time.h>
#include <errno.h>
#include <stdlib.h>
#include <stdatomic.h>

static long long hpt_clock_now(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (long long)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

static void hpt_sleep_until_impl(long long deadline_ns) {
	struct timespec ts;
	ts.tv_sec  = deadline_ns / 1000000000LL;
	ts.tv_nsec = deadline_ns % 1000000000LL;
	while (clock_nanosleep(CLOCK_MONOTONIC, TIMER_ABSTIME, &ts, NULL) == EINTR) {}
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
	long long tick = 0;
	while (!atomic_load(&t->stop)) {
		tick++;
		hpt_sleep_until_impl(t->start_ns + tick * t->period_ns);
		if (atomic_load(&t->stop)) break;
		char b = 1;
		write(t->pipe_w, &b, 1);
	}
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
		// Pipe creation failed — fall back to goroutine loop.
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
		// pipeR.Read returns error once the C thread closes the write end,
		// causing the goroutine to exit and close pipeR.
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
