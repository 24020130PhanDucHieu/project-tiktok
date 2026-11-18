package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
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

func rateLimitedThenOK(failures int, hits *int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if int(atomic.AddInt32(hits, 1)) <= failures {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"code":36009002,"message":"Too many requests"}`))
			return
		}
		w.Write([]byte(`{"code":0,"message":"Success","data":{"ok":true}}`))
	}
}

func liveService(t *testing.T, h http.HandlerFunc) *TTShopService {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	store := &memTokens{}
	s := serviceWithStore(t, srv.URL, store)
	store.tok = &domain.TTSToken{Access: sealed(t, s, "a"), Refresh: sealed(t, s, "r"),
		AccessExpireAt: time.Now().Add(time.Hour), RefreshExpireAt: time.Now().Add(24 * time.Hour)}
	return s
}

func TestReadIsRetriedOnceWhenRateLimited(t *testing.T) {
	var hits int32
	s := liveService(t, rateLimitedThenOK(1, &hits))
	if _, err := s.get(context.Background(), "/x/y", nil); err != nil {
		t.Fatalf("the retry should have succeeded: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
	if _, err := s.pace.reserve(); err != nil {
		t.Fatalf("a recovered read must not pause later calls: %v", err)
	}
}

func TestStillRateLimitedAfterRetryPausesCalls(t *testing.T) {
	var hits int32
	s := liveService(t, rateLimitedThenOK(100, &hits))
	if _, err := s.get(context.Background(), "/x/y", nil); !IsRateLimited(err) {
		t.Fatalf("expected a rate limit error, got %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2 (one try and one retry)", hits)
	}
	before := hits
	_, err := s.get(context.Background(), "/x/y", nil)
	var rl *ErrRateLimited
	if !errors.As(err, &rl) || hits != before {
		t.Fatalf("during the pause nothing may be sent: err %v, hits %d -> %d", err, before, hits)
	}
}

func TestWriteIsNeverRetried(t *testing.T) {
	var hits int32
	s := liveService(t, rateLimitedThenOK(100, &hits))
	_, err := s.callWrite(context.Background(), http.MethodPost, "/affiliate_seller/202508/target_collaborations", nil, map[string]string{})
	if !IsRateLimited(err) || hits != 1 {
		t.Fatalf("a write must be sent once: err %v, hits %d", err, hits)
	}
}

func TestTheDefaultPacerLeavesFiveToTenSecondsBetweenTwoCalls(t *testing.T) {
	c := &fakeClock{t: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)}
	p := &pacer{now: c.now} // the defaults
	if wait, err := p.reserve(); err != nil || wait != 0 {
		t.Fatalf("the first call of an idle pacer starts at once: %v %v", wait, err)
	}
	seen := map[time.Duration]bool{}
	prev := time.Duration(0)
	for i := 0; i < 30; i++ {
		wait, err := p.reserve()
		if err != nil {
			t.Fatal(err)
		}
		gap := wait - prev // each booked slot is one gap after the one before
		if gap < 5*time.Second || gap > 10*time.Second {
			t.Fatalf("call %d: the gap to the previous call is %v, want 5 to 10 seconds", i+2, gap)
		}
		seen[gap] = true
		prev = wait
	}
	if len(seen) < 2 {
		t.Fatal("the gap is drawn at random, not fixed")
	}
}

func TestTheSearchOfCreatorsIsNotTriedASecondTimeWhenRateLimitedButStillPausesTheCalls(t *testing.T) {
	var hits int32
	s := liveService(t, rateLimitedThenOK(100, &hits))
	_, err := s.postSearch(context.Background(), ttsCreatorSearchPath, nil, map[string]string{"keyword": "abc"})
	if !IsRateLimited(err) {
		t.Fatalf("a refused search is reported as rate limited: %v", err)
	}
	if hits != 1 {
		t.Fatalf("the creator search is sent once: a retry two seconds later would only be refused too, hits = %d", hits)
	}
	if _, err := s.pace.reserve(); !IsRateLimited(err) {
		t.Fatalf("the refusal still pauses every call to TikTok: %v", err)
	}
}

func TestOtherSearchesAndReadsAreStillTriedASecondTime(t *testing.T) {
	var hits int32
	s := liveService(t, rateLimitedThenOK(1, &hits))
	if _, err := s.postSearch(context.Background(), ttsProductSearchPath, nil, map[string]string{"status": "ACTIVATE"}); err != nil {
		t.Fatalf("a product search is retried and the retry works: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
	if retriesAfterRateLimit("POST", ttsCreatorSearchPath) || !retriesAfterRateLimit("GET", "/x/y") || retriesAfterRateLimit("POST", "/orders/create") {
		t.Fatal("only reads are retried, and not the creator search")
	}
}

func TestABackgroundCallBooksASlotOnlyWhenItIsFreeNow(t *testing.T) {
	p, _ := newTestPacer(time.Second, 100)
	if wait, booked, err := p.reserveBackground(); err != nil || !booked || wait != 0 {
		t.Fatalf("an idle pacer gives the slot at once: %v %v %v", wait, booked, err)
	}
	next := p.next
	wait, booked, err := p.reserveBackground()
	if err != nil || booked || wait != time.Second {
		t.Fatalf("the slot is taken: wait for it and ask again, booking nothing: %v %v %v", wait, booked, err)
	}
	if !p.next.Equal(next) || len(p.sent) != 1 {
		t.Fatalf("waiting books nothing: next moved to %v, %d calls recorded", p.next, len(p.sent))
	}
}

func TestAPersonWhoAsksWhileBackgroundWorkWaitsGoesFirst(t *testing.T) {
	p, c := newTestPacer(time.Second, 100)
	if _, err := p.reserve(); err != nil { // somebody's call takes the slot of now
		t.Fatal(err)
	}
	bgWait, booked, _ := p.reserveBackground() // the import wants to go: nothing is free for a second
	if booked || bgWait != time.Second {
		t.Fatalf("background work waits: %v %v", bgWait, booked)
	}
	personWait, err := p.reserve() // a person asks in the meantime: the next slot is theirs
	if err != nil || personWait != time.Second {
		t.Fatalf("the person books the slot the import was waiting for: %v %v", personWait, err)
	}
	c.add(time.Second) // the import asks again
	again, booked, _ := p.reserveBackground()
	if booked || again != time.Second {
		t.Fatalf("the slot is the person's, so the import waits another gap: %v %v", again, booked)
	}
	c.add(time.Second)
	if _, booked, _ := p.reserveBackground(); !booked {
		t.Fatal("the import gets the next free slot")
	}
}

func TestBackgroundWorkGivesWayInRealTimeToAPersonWhoComesLater(t *testing.T) {
	p := &pacer{gap: 60 * time.Millisecond, perHour: 100}
	if err := p.wait(context.Background()); err != nil { // the slot of now is taken
		t.Fatal(err)
	}
	done := make(chan string, 2)
	go func() {
		_ = p.wait(WithBackgroundPriority(context.Background()))
		done <- "background"
	}()
	time.Sleep(10 * time.Millisecond) // the import is waiting for its turn
	go func() {
		_ = p.wait(context.Background())
		done <- "person"
	}()
	first, second := <-done, <-done
	if first != "person" || second != "background" {
		t.Fatalf("the person who came later is served first: %s then %s", first, second)
	}
}

func TestBackgroundPriorityFollowsTheContextAndRespectsThePauseAndTheCeiling(t *testing.T) {
	if isBackground(context.Background()) || !isBackground(WithBackgroundPriority(context.Background())) {
		t.Fatal("the marker is on the context")
	}
	p, _ := newTestPacer(time.Second, 100)
	p.penalize(30 * time.Second)
	if _, _, err := p.reserveBackground(); !IsRateLimited(err) {
		t.Fatalf("background work is paused with everything else: %v", err)
	}
	q, _ := newTestPacer(time.Millisecond, 1)
	if _, booked, _ := q.reserveBackground(); !booked {
		t.Fatal("the first call goes")
	}
	if _, _, err := q.reserveBackground(); !IsRateLimited(err) {
		t.Fatalf("the hourly ceiling applies to background work too: %v", err)
	}
}

func TestOnlyTheHeadersThatTalkAboutLimitsAreLogged(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "3")
	h.Set("Retry-After", "45")
	h.Set("X-Rate-Limit-Reset", "1700000000")
	h.Set("Set-Cookie", "session=secret")
	h.Set("Authorization", "Bearer secret")
	h.Set("Content-Type", "application/json")
	got := rateLimitHeaders(h)
	want := "retry-after=45; x-rate-limit-reset=1700000000; x-ratelimit-remaining=3"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if rateLimitHeaders(http.Header{"Content-Type": {"x"}}) != "" {
		t.Fatal("no limit headers, nothing logged")
	}
}

func TestTheBackgroundMarkerSurvivesTheCacheThatRunsTheLoadWithoutTheCallersCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(WithBackgroundPriority(context.Background()))
	cancel()
	if !isBackground(context.WithoutCancel(ctx)) {
		t.Fatal("the read cache loads with WithoutCancel: the marker must stay, or the import would book slots like a person")
	}
}
