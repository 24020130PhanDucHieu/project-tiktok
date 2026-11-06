package service

import (
	"context"
	"net/http"
	"strings"
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
