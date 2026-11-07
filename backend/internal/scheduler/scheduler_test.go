package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/service"
)

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A server that restarts more often than the sweep interval (a deploy every few hours) used to never run the sweep:
// the first run came only after one whole interval.
func TestAJobRunsSoonAfterStartWhenItHasAFirstDelay(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go (&Scheduler{}).every(ctx, 20*time.Millisecond, time.Hour, func(context.Context) { runs.Add(1) })

	waitFor(t, "the first run", func() bool { return runs.Load() == 1 })
}

func TestAJobWithoutAFirstDelayWaitsForItsInterval(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go (&Scheduler{}).every(ctx, 0, time.Hour, func(context.Context) { runs.Add(1) })

	time.Sleep(100 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatalf("the digest check must not fire the moment the server starts (it would send twice after a restart), ran %d times", runs.Load())
	}
}

func TestAJobKeepsTickingAfterItsFirstRun(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go (&Scheduler{}).every(ctx, 5*time.Millisecond, 20*time.Millisecond, func(context.Context) { runs.Add(1) })

	waitFor(t, "the first run and two ticks", func() bool { return runs.Load() >= 3 })
}

// The sweep no longer stops at a failing KOC, so the job record must still show that some were not served.
func TestASweepThatSkippedKocsIsRecordedAsFailed(t *testing.T) {
	if err := sweepError(&service.SweepSummary{}, nil); err != nil {
		t.Fatalf("a clean sweep is not an error: %v", err)
	}
	err := sweepError(&service.SweepSummary{Failures: 2, FirstError: "follow-up reminder @linh: connection reset"}, nil)
	if err == nil || !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "@linh") {
		t.Fatalf("skipped KOCs must show in the job record with the first cause, got %v", err)
	}
	boom := errors.New("cannot list KOCs")
	if got := sweepError(nil, boom); got != boom {
		t.Fatalf("a real error stays as it is, got %v", got)
	}
}

func TestAJobStopsWhenTheContextEnds(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		(&Scheduler{}).every(ctx, time.Hour, time.Hour, func(context.Context) { runs.Add(1) })
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the loop did not stop with its context")
	}
	if runs.Load() != 0 {
		t.Fatalf("a cancelled job must not run, ran %d times", runs.Load())
	}
}
