package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTheFirstMainImageOfAProductIsRead(t *testing.T) {
	raw := []byte(`{"id":"1","title":"Áo","main_images":[{"urls":["https://p16.tiktokcdn.com/a.jpeg","https://p19/b.jpeg"],"thumb_urls":["https://p16/a-thumb.jpeg"]},{"urls":["https://x/2.jpeg"]}]}`)
	if got := parseProductImage(raw); got != "https://p16.tiktokcdn.com/a.jpeg" {
		t.Fatalf("the first picture: %q", got)
	}
	if got := parseProductImage([]byte(`{"main_images":[{"thumb_urls":["https://t/1.jpeg"]}]}`)); got != "https://t/1.jpeg" {
		t.Fatalf("a thumbnail is better than nothing: %q", got)
	}
	for _, bad := range []string{`{}`, `{"main_images":[]}`, `{"main_images":[{"urls":[]}]}`, `not json`, `{"main_images":[{"urls":["javascript:alert(1)"]}]}`, `{"main_images":[{"urls":["http://insecure/a.jpg"]}]}`} {
		if got := parseProductImage([]byte(bad)); got != "" {
			t.Fatalf("%s must give no picture, got %q", bad, got)
		}
	}
}

func TestProductPicturesAreFetchedOnceAndKept(t *testing.T) {
	s := newTTShopForTest("")
	var calls atomic.Int32
	fetch := func(_ context.Context, id string) (string, error) {
		calls.Add(1)
		return "https://img/" + id + ".jpg", nil
	}
	ctx := context.Background()

	got := s.productImages(ctx, []string{"1736294693654136704", "1736294693654136704", " 1736613031755089792 ", "abc", "../x", ""}, fetch)
	if len(got) != 2 || got["1736294693654136704"] != "https://img/1736294693654136704.jpg" || got["1736613031755089792"] == "" {
		t.Fatalf("each valid id once, the others dropped: %v", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("one call per distinct product, got %d", calls.Load())
	}
	s.productImages(ctx, []string{"1736294693654136704", "1736613031755089792"}, fetch)
	if calls.Load() != 2 {
		t.Fatalf("a picture already read is not asked for again, got %d calls", calls.Load())
	}
}

func TestAProductWhosePictureCannotBeReadDoesNotFailTheOthers(t *testing.T) {
	s := newTTShopForTest("")
	var calls atomic.Int32
	fetch := func(_ context.Context, id string) (string, error) {
		calls.Add(1)
		if id == "2" {
			return "", errors.New("too many requests")
		}
		return "https://img/" + id + ".jpg", nil
	}
	got := s.productImages(context.Background(), []string{"1", "2", "3"}, fetch)
	if got["1"] == "" || got["3"] == "" || got["2"] != "" {
		t.Fatalf("the readable ones come back, the failing one has none: %v", got)
	}
	s.productImages(context.Background(), []string{"2"}, func(_ context.Context, id string) (string, error) {
		calls.Add(1)
		return "https://img/2.jpg", nil
	})
	if got := s.productImages(context.Background(), []string{"2"}, fetch); got["2"] != "https://img/2.jpg" {
		t.Fatalf("a failure is not remembered, the next try reads it: %v", got)
	}
}

func TestProductPicturesAreFetchedFewAtATimeAndNotMoreThanAskedFor(t *testing.T) {
	s := newTTShopForTest("")
	var running, peak atomic.Int32
	var mu sync.Mutex
	asked := 0
	fetch := func(_ context.Context, id string) (string, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		mu.Lock()
		asked++
		mu.Unlock()
		running.Add(-1)
		return "https://img/" + id, nil
	}
	ids := make([]string, 60)
	for i := range ids {
		ids[i] = strconv.Itoa(1000 + i)
	}
	got := s.productImages(context.Background(), ids, fetch)
	if len(got) != productImagesPerCall || asked != productImagesPerCall {
		t.Fatalf("at most %d products per call, got %d pictures after %d reads", productImagesPerCall, len(got), asked)
	}
	if peak.Load() > productImageWorkers {
		t.Fatalf("TikTok shares one rate limit: at most %d reads at once, saw %d", productImageWorkers, peak.Load())
	}
}
