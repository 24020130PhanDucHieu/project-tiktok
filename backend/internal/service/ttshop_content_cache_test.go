package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func affLine(id string, at time.Time) AffiliateLine {
	return AffiliateLine{OrderID: id, CreatedAt: at}
}

func newCachedShop(fetch func(ctx context.Context, from, to time.Time) (affCacheEntry, error)) *TTShopService {
	s := &TTShopService{}
	s.affFetch = fetch
	return s
}

func TestAffiliateLinesAreKeptForAFewMinutes(t *testing.T) {
	var calls atomic.Int32
	now := time.Now()
	s := newCachedShop(func(_ context.Context, from, to time.Time) (affCacheEntry, error) {
		calls.Add(1)
		return affCacheEntry{from: from, to: to, at: time.Now(), lines: []AffiliateLine{affLine("a", now.Add(-time.Hour))}}, nil
	})
	from := now.Add(-48 * time.Hour)
	for i := 0; i < 3; i++ {
		lines, _, err := s.AffiliateLines(context.Background(), from, time.Now())
		if err != nil || len(lines) != 1 {
			t.Fatalf("%v %v", lines, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("three reads inside the TTL must cost one TikTok read, got %d", calls.Load())
	}
}

func TestAShorterRangeIsServedFromTheLongerOneKept(t *testing.T) {
	var calls atomic.Int32
	now := time.Now()
	s := newCachedShop(func(_ context.Context, from, to time.Time) (affCacheEntry, error) {
		calls.Add(1)
		return affCacheEntry{from: from, to: to, at: time.Now(), lines: []AffiliateLine{
			affLine("old", now.Add(-60*24*time.Hour)), affLine("new", now.Add(-2*time.Hour)),
		}}, nil
	})
	if _, _, err := s.AffiliateLines(context.Background(), now.Add(-90*24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	lines, _, err := s.AffiliateLines(context.Background(), now.Add(-30*24*time.Hour), now)
	if err != nil || len(lines) != 1 || lines[0].OrderID != "new" {
		t.Fatalf("a 30-day read must come from the 90-day answer, cut to 30 days: %v %v", lines, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("no second TikTok read expected, got %d", calls.Load())
	}
}

func TestSimultaneousAffiliateReadsShareOneLoad(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	now := time.Now()
	s := newCachedShop(func(_ context.Context, from, to time.Time) (affCacheEntry, error) {
		calls.Add(1)
		<-release
		return affCacheEntry{from: from, to: to, at: time.Now(), lines: []AffiliateLine{affLine("a", now)}}, nil
	})
	from := now.Add(-24 * time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.AffiliateLines(context.Background(), from, time.Now()); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("five readers at once must share one load, got %d", calls.Load())
	}
}

func TestAStaleAnswerIsServedAtOnceAndRefreshedInTheBackground(t *testing.T) {
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	refreshed := make(chan struct{})
	release := make(chan struct{})
	s := newCachedShop(func(_ context.Context, f, to time.Time) (affCacheEntry, error) {
		<-release
		defer close(refreshed)
		return affCacheEntry{from: f, to: to, at: time.Now(), lines: []AffiliateLine{affLine("fresh", now)}}, nil
	})
	s.affCache = []affCacheEntry{{from: from, to: now.Add(-10 * time.Minute), at: now.Add(-10 * time.Minute), lines: []AffiliateLine{affLine("stale", now.Add(-time.Hour))}}}

	start := time.Now()
	lines, _, err := s.AffiliateLines(context.Background(), from, now)
	if err != nil || len(lines) != 1 || lines[0].OrderID != "stale" {
		t.Fatalf("the stale answer must be returned: %v %v", lines, err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("a stale answer must not wait for TikTok")
	}
	close(release)
	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("the background refresh never ran")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		e, _, _ := s.affLookup(from)
		if len(e.lines) == 1 && e.lines[0].OrderID == "fresh" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the refreshed answer was not kept")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAnAnswerOlderThanTheStaleLimitIsLoadedAgainAndFailuresAreNotKept(t *testing.T) {
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	boom := errors.New("tiktok down")
	s := newCachedShop(func(context.Context, time.Time, time.Time) (affCacheEntry, error) { return affCacheEntry{}, boom })
	s.affCache = []affCacheEntry{{from: from, to: now, at: now.Add(-affOrdersStale - time.Minute), lines: []AffiliateLine{affLine("ancient", now)}}}

	if _, _, err := s.AffiliateLines(context.Background(), from, now); !errors.Is(err, boom) {
		t.Fatalf("a too-old answer must not be served; the load error is returned, got %v", err)
	}
	if e, _, _ := s.affLookup(from); len(e.lines) != 1 {
		t.Fatal("a failed load must leave the kept answer alone")
	}
}
