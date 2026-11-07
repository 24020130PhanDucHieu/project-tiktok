package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/service"
)

// manyVideos gives the creator n videos, one order each, newest first.
func manyVideos(e *contentEnv, n int) {
	now := time.Now()
	e.tts.lines = nil
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("76893112924436%05d", 70000-i) // snowflake-like: the date part is far in the past, ids stay unique
		e.tts.lines = append(e.tts.lines, line(fmt.Sprintf("o%d", i), "changditap1802", id, "111", now.Add(-time.Duration(i+1)*time.Hour), 10000, "SETTLED"))
	}
}

func idsOf(body map[string]any) []string {
	var out []string
	for _, v := range dataOf(body)["videos"].([]any) {
		out = append(out, v.(map[string]any)["id"].(string))
	}
	return out
}

func TestContentVideosPagesCutTheListAndKeepTheTotal(t *testing.T) {
	e := newContentEnv()
	manyVideos(e, 7)

	_, p1 := e.get(t, "/content/videos?limit=3&page=1")
	_, p2 := e.get(t, "/content/videos?limit=3&page=2")
	_, p3 := e.get(t, "/content/videos?limit=3&page=3")
	_, p4 := e.get(t, "/content/videos?limit=3&page=4")

	if d := dataOf(p1); d["total"].(float64) != 7 || d["page"].(float64) != 1 || d["pageSize"].(float64) != 3 {
		t.Fatalf("total, page and pageSize must be reported: %v", d)
	}
	a, b, c := idsOf(p1), idsOf(p2), idsOf(p3)
	if len(a) != 3 || len(b) != 3 || len(c) != 1 || len(idsOf(p4)) != 0 {
		t.Fatalf("pages must hold 3,3,1,0 videos: %d %d %d %d", len(a), len(b), len(c), len(idsOf(p4)))
	}
	seen := map[string]bool{}
	for _, id := range append(append(a, b...), c...) {
		if seen[id] {
			t.Fatalf("a video must not appear on two pages: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 7 {
		t.Fatalf("the pages together must cover every video: %d", len(seen))
	}
}

func TestContentVideosKeywordFiltersBeforeThePageIsCut(t *testing.T) {
	e := newContentEnv()
	manyVideos(e, 6)
	// a video of another creator that only the keyword can find, deep in the list
	e.tts.lines = append(e.tts.lines, line("ox", "other.kol", "7689311292443699999", "111", time.Now().Add(-100*time.Hour), 1, "SETTLED"))

	_, body := e.get(t, "/content/videos?all=1&limit=2&page=1&q=other.kol")
	d := dataOf(body)
	if d["total"].(float64) != 1 || len(idsOf(body)) != 1 || idsOf(body)[0] != "7689311292443699999" {
		t.Fatalf("the keyword must narrow the whole list, not just the first page: %v", d)
	}
}

func TestContentVideosOnlyReadsVideoDetailsOfTheShownPage(t *testing.T) {
	e := newContentEnv()
	manyVideos(e, 8)
	e.tts.videosErr = nil
	e.tts.videos = []service.AnalyticsVideo{}
	e.tts.traffic = &service.VideoTraffic{Views: 5, Likes: 1}

	e.get(t, "/content/videos?limit=2&page=2")
	if got := len(e.tts.trafficIDs); got != 2 {
		t.Fatalf("only the 2 videos of the shown page may be enriched, got %d", got)
	}
}

func TestContentVideosBadPageFallsBackToTheFirst(t *testing.T) {
	e := newContentEnv()
	manyVideos(e, 3)
	_, body := e.get(t, "/content/videos?limit=2&page=-4")
	if dataOf(body)["page"].(float64) != 1 || len(idsOf(body)) != 2 {
		t.Fatalf("a bad page number means page 1: %v", dataOf(body))
	}
}
