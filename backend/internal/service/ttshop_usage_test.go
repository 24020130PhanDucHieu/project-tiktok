package service

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestUsageCountsCallsByHourAndKindAndTheOnesTikTokRefused(t *testing.T) {
	lines := []string{
		"2026-11-12T16:10:00Z POST /affiliate_seller/202410/orders/search q=page_size=100 body={} http=200 code=0 result=ok msg=\"Success\" request_id=1 bytes=10 ms=100",
		"2026-11-12T16:11:00Z POST /affiliate_seller/202410/orders/search q=page_size=100&page_token=<set> body={} http=200 code=0 result=ok msg=\"Success\" request_id=2 bytes=10 ms=100",
		"2026-11-12T17:01:00Z GET /affiliate_seller/202508/target_collaborations/7707325607445366549 http=200 code=0 result=ok msg=\"Success\" request_id=3 bytes=10 ms=100",
		"2026-11-12T17:02:00Z POST /affiliate_seller/202508/target_collaborations/search q=page_size=20 body={} http=429 code=36009002 result=rejected msg=\"Too many requests\" request_id=4 bytes=10 ms=100",
		"2026-11-12T17:03:00Z POST /affiliate_seller/202508/target_collaborations/search q=page_size=20 body={} http=200 code=36009002 result=rejected msg=\"Too many requests\" request_id=5 bytes=10 ms=100",
		"2026-11-11T01:00:00Z GET /old http=200 code=0 result=ok",
		"not a log line",
	}
	rep := parseUsage(lines, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if rep.Calls != 5 || rep.Rejected != 2 {
		t.Fatalf("5 calls since the start, 2 refused: %+v", rep)
	}
	if len(rep.Hours) != 2 || rep.Hours[0] != (UsageHour{Hour: "2026-11-12T16", Calls: 2}) || rep.Hours[1] != (UsageHour{Hour: "2026-11-12T17", Calls: 3, Rejected: 2}) {
		t.Fatalf("hours: %+v", rep.Hours)
	}
	if rep.Kinds[0].Kind != "POST /affiliate_seller/{id}/orders/search" || rep.Kinds[0].Calls != 2 {
		t.Fatalf("the versions and ids of one call are one kind: %+v", rep.Kinds)
	}
	for _, k := range rep.Kinds {
		if k.Kind == "POST /affiliate_seller/{id}/target_collaborations/search" && (k.Calls != 2 || k.Rejected != 2) {
			t.Fatalf("%+v", k)
		}
		if strings.Contains(k.Kind, "7707") {
			t.Fatalf("an id leaked into a kind: %q", k.Kind)
		}
	}
	if rep.From != "2026-11-12T16:10:00Z" || rep.To != "2026-11-12T17:03:00Z" {
		t.Fatalf("range %s .. %s", rep.From, rep.To)
	}
}

func TestUsageReadsTheLogFileAndItsPreviousCopy(t *testing.T) {
	path := t.TempDir() + "/logs/ttshop.log"
	l := newTTSLog(path)
	t.Cleanup(l.close)
	s := &TTShopService{logs: l}
	l.add("GET /product/202309/products/1736294693654136704 http=200 code=0 result=ok")
	l.size = ttsLogMaxFile + 1 // the next write rotates the file
	l.add("POST /affiliate_seller/202410/orders/search http=200 code=0 result=ok")
	l.add("POST /affiliate_seller/202410/orders/search http=429 code=36009002 result=rejected")
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("the full file is kept as .1: %v", err)
	}
	rep := s.Usage(1)
	if rep.Calls != 3 || rep.Rejected != 1 || !rep.Complete {
		t.Fatalf("both files are counted: %+v", rep)
	}
}

func TestTheLogFileRotatesWhileRunning(t *testing.T) {
	path := t.TempDir() + "/ttshop.log"
	l := newTTSLog(path)
	t.Cleanup(l.close)
	l.add("GET /a http=200 code=0 result=ok")
	l.size = ttsLogMaxFile + 1
	l.add("GET /b http=200 code=0 result=ok") // goes to the file that is then rotated
	l.add("GET /c http=200 code=0 result=ok") // starts the new file
	old, _ := os.ReadFile(path + ".1")
	cur, _ := os.ReadFile(path)
	if strings.Count(string(old), "\n") != 2 || strings.Count(string(cur), "\n") != 1 || !strings.Contains(string(cur), "GET /c") {
		t.Fatalf("old: %q current: %q", old, cur)
	}
}

func TestUsageWithoutAFileUsesWhatTheProcessRemembers(t *testing.T) {
	l := newTTSLog("")
	s := &TTShopService{logs: l}
	l.add("GET /x http=200 code=0 result=ok")
	if rep := s.Usage(24); rep.Calls != 1 {
		t.Fatalf("%+v", rep)
	}
}
