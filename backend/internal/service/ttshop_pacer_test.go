package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) now() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.t }
func (f *fakeClock) add(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

func newTestPacer(gap time.Duration, perHour int) (*pacer, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)}
	return &pacer{now: c.now, gap: gap, perHour: perHour}, c
}

func TestPacerSpacesCalls(t *testing.T) {
	p, _ := newTestPacer(time.Second, 100)
	want := []time.Duration{0, time.Second, 2 * time.Second}
	for i, w := range want {
		got, err := p.reserve()
		if err != nil || got != w {
			t.Fatalf("call %d: wait %v err %v, want %v", i, got, err, w)
		}
	}
}

func TestPacerNoWaitAfterIdle(t *testing.T) {
	p, c := newTestPacer(time.Second, 100)
	p.reserve()
	c.add(10 * time.Second)
	if got, err := p.reserve(); err != nil || got != 0 {
		t.Fatalf("wait %v err %v, want 0", got, err)
	}
}

func TestPacerHourlyCeilingSlides(t *testing.T) {
	p, c := newTestPacer(0, 3)
	p.gap = time.Nanosecond
	for i := 0; i < 3; i++ {
		if _, err := p.reserve(); err != nil {
			t.Fatal(err)
		}
		c.add(10 * time.Minute)
	}
	_, err := p.reserve()
	var rl *ErrRateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if rl.RetryAfter != 30*time.Minute {
		t.Fatalf("retry after %v, want 30m (the oldest call leaves the window)", rl.RetryAfter)
	}
	c.add(30*time.Minute + time.Second)
	if _, err := p.reserve(); err != nil {
		t.Fatalf("window should have slid: %v", err)
	}
}

func TestPacerPenaltyPausesEverything(t *testing.T) {
	p, c := newTestPacer(time.Millisecond, 100)
	p.penalize(45 * time.Second)
	_, err := p.reserve()
	var rl *ErrRateLimited
	if !errors.As(err, &rl) || rl.RetryAfter != 45*time.Second {
		t.Fatalf("expected a 45s pause, got %v", err)
	}
	if !IsRateLimited(err) {
		t.Fatal("IsRateLimited must recognise the pause")
	}
	c.add(46 * time.Second)
	if _, err := p.reserve(); err != nil {
		t.Fatalf("pause should be over: %v", err)
	}
}

func TestIsRateLimitedRecognisesTikTokCode(t *testing.T) {
	if !IsRateLimited(&TTSRejected{Code: 36009002}) {
		t.Fatal("36009002 is the rate limit")
	}
	if IsRateLimited(&TTSRejected{Code: 28001022}) || IsRateLimited(nil) {
		t.Fatal("other errors are not rate limits")
	}
}

func TestPacerWaitHonoursContext(t *testing.T) {
	p := &pacer{gap: time.Hour, perHour: 10}
	if err := p.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestPacerConcurrentCallsGetDistinctSlots(t *testing.T) {
	p, _ := newTestPacer(time.Second, 1000)
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[time.Duration]bool{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, _ := p.reserve()
			mu.Lock()
			seen[d] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Fatalf("20 concurrent calls got %d distinct slots", len(seen))
	}
}
