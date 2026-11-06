package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// listService answers the invitation search with the next scripted reply; replies repeat the last one.
func listService(t *testing.T, hits *int32, replies ...string) *TTShopService {
	t.Helper()
	s := liveService(t, func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(hits, 1)) - 1
		if i >= len(replies) {
			i = len(replies) - 1
		}
		if strings.Contains(replies[i], "36009002") {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		w.Write([]byte(replies[i]))
	})
	s.cipher, s.cipherAt = "cipher", time.Now()
	return s
}

const (
	listOK      = `{"code":0,"message":"Success","data":{"total_count":1,"target_collaborations":[{"id":"1","name":"A"}]}}`
	listLimited = `{"code":36009002,"message":"Too many requests"}`
)

func TestTargetListIsServedFromCacheWhileFresh(t *testing.T) {
	var hits int32
	s := listService(t, &hits, listOK)
	for i := 0; i < 3; i++ {
		page, err := s.SearchTargets(context.Background(), TargetListParams{})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("call %d: %v %+v", i, err, page)
		}
	}
	if hits != 1 {
		t.Fatalf("TikTok was asked %d times, want 1", hits)
	}
	if _, err := s.SearchTargets(context.Background(), TargetListParams{Keyword: "x"}); err != nil || hits != 2 {
		t.Fatalf("another filter is another list: err %v hits %d", err, hits)
	}
}

func TestStaleListIsShownWhenTikTokRefuses(t *testing.T) {
	var hits int32
	s := listService(t, &hits, listOK, listLimited)
	if _, err := s.SearchTargets(context.Background(), TargetListParams{}); err != nil {
		t.Fatal(err)
	}
	// age the entry so it must be refreshed
	s.targets.mu.Lock()
	for k, e := range s.targets.m {
		e.at = e.at.Add(-targetListFresh - time.Second)
		s.targets.m[k] = e
	}
	s.targets.mu.Unlock()
	page, err := s.SearchTargets(context.Background(), TargetListParams{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the last list should be shown instead of an error: %v %+v", err, page)
	}
}

func TestRateLimitedWithNothingCachedIsAnError(t *testing.T) {
	var hits int32
	s := listService(t, &hits, listLimited)
	if _, err := s.SearchTargets(context.Background(), TargetListParams{}); !IsRateLimited(err) {
		t.Fatalf("expected a rate limit error, got %v", err)
	}
}

func TestBadFilterIsNotCachedOrSent(t *testing.T) {
	var hits int32
	s := listService(t, &hits, listOK)
	if _, err := s.SearchTargets(context.Background(), TargetListParams{Status: "NOPE"}); err == nil || hits != 0 {
		t.Fatalf("err %v hits %d", err, hits)
	}
}

func TestForgetClearsTheLists(t *testing.T) {
	var hits int32
	s := listService(t, &hits, listOK)
	s.SearchTargets(context.Background(), TargetListParams{})
	s.targets.forget()
	s.SearchTargets(context.Background(), TargetListParams{})
	if hits != 2 {
		t.Fatalf("hits = %d, want 2 after forget", hits)
	}
}

func TestOpenCollabsAreCachedAndFallBackToTheLastPage(t *testing.T) {
	var hits int32
	ok := `{"code":0,"message":"Success","data":{"open_collaborations":[]}}`
	s := listService(t, &hits, ok, listLimited)
	if _, err := s.OpenCollaborations(context.Background(), "", "", 6); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt32(&hits) // the page may need a few extra reads (prices); the repeats must add none
	for i := 0; i < 2; i++ {
		if _, err := s.OpenCollaborations(context.Background(), "", "", 6); err != nil {
			t.Fatal(err)
		}
	}
	if hits != afterFirst {
		t.Fatalf("repeats asked TikTok again: hits %d -> %d", afterFirst, hits)
	}
	s.openCollabs.mu.Lock()
	for k, e := range s.openCollabs.m {
		e.at = e.at.Add(-openCollabFresh - time.Second)
		s.openCollabs.m[k] = e
	}
	s.openCollabs.mu.Unlock()
	if _, err := s.OpenCollaborations(context.Background(), "", "", 6); err != nil {
		t.Fatalf("the last page should be shown while TikTok refuses: %v", err)
	}
}

type memReadStore struct {
	mu    sync.Mutex
	items map[string]memReadItem
}
type memReadItem struct {
	data []byte
	at   time.Time
}

func (m *memReadStore) Load(_ context.Context, key string) ([]byte, time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[key]
	return it.data, it.at, ok, nil
}
func (m *memReadStore) Save(_ context.Context, key string, data []byte, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = map[string]memReadItem{}
	}
	m.items[key] = memReadItem{data: data, at: at}
	return nil
}

func TestCachedAnswersSurviveARestart(t *testing.T) {
	store := &memReadStore{}
	var hits int32
	first := listService(t, &hits, listOK)
	first.WithReadCache(store)
	if _, err := first.SearchTargets(context.Background(), TargetListParams{}); err != nil {
		t.Fatal(err)
	}
	if hits != 1 || len(store.items) != 1 {
		t.Fatalf("hits %d stored %d", hits, len(store.items))
	}

	// a new process: empty memory, same store, and TikTok now refusing everything
	var hits2 int32
	second := listService(t, &hits2, listLimited)
	second.WithReadCache(store)
	page, err := second.SearchTargets(context.Background(), TargetListParams{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the stored list should answer after a restart: %v %+v", err, page)
	}
	if hits2 != 0 {
		t.Fatalf("a fresh stored answer must not call TikTok, hits = %d", hits2)
	}
}

func TestVideoTrafficIsKeptLongerForOldVideos(t *testing.T) {
	now := time.Now()
	snowflake := func(age time.Duration) string { return strconv.FormatUint(uint64(now.Add(-age).Unix())<<32, 10) }
	young, mid, old := trafficFresh(snowflake(time.Hour), now), trafficFresh(snowflake(7*24*time.Hour), now), trafficFresh(snowflake(40*24*time.Hour), now)
	if !(young < mid && mid < old) {
		t.Fatalf("freshness should grow with the video's age: %v %v %v", young, mid, old)
	}
	if trafficFresh("123", now) != young {
		t.Fatal("an id with no date is treated as new")
	}
}

func TestCreatorLookupIsCached(t *testing.T) {
	var hits int32
	s := listService(t, &hits, `{"code":0,"message":"Success","data":{"creators":[]}}`)
	for i := 0; i < 3; i++ {
		if _, err := s.SearchCreators(context.Background(), "Abc", 12); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SearchCreators(context.Background(), "abc", 12); err != nil || hits != 1 {
		t.Fatalf("same keyword (any case) must be one call: err %v hits %d", err, hits)
	}
}
