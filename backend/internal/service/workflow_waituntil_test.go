package service

import (
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestNextClockTime(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 20, h, m, 0, 0, vnZone) }
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"before the time today", at(8, 0), at(9, 0)},
		{"after the time today goes to tomorrow", at(10, 0), at(9, 0).AddDate(0, 0, 1)},
		{"exactly at the time goes to tomorrow", at(9, 0), at(9, 0).AddDate(0, 0, 1)},
		{"one minute before", at(8, 59), at(9, 0)},
		{"late at night", at(23, 59), at(9, 0).AddDate(0, 0, 1)},
		{"just after midnight", at(0, 1), at(9, 0)},
	}
	for _, tc := range cases {
		if got := nextClockTime(tc.now, 9*60); !got.Equal(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := nextClockTime(at(10, 0), 0); !got.Equal(time.Date(2026, 11, 21, 0, 0, 0, 0, vnZone)) {
		t.Errorf("midnight is the next midnight: %v", got)
	}
	// the time is Vietnam time whatever zone the clock is in
	utc := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC) // 10:00 in Vietnam
	if got := nextClockTime(utc, 9*60); !got.Equal(at(9, 0).AddDate(0, 0, 1)) {
		t.Errorf("a clock in UTC: %v", got)
	}
}

func waitUntilGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	until := domain.WorkflowNode{ID: "u", Type: domain.NodeWaitUntil, AtMinute: 9 * 60}
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 1), until, wwait("w2", 5)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "w"), wedge("w", domain.PortOut, "u"), wedge("u", domain.PortOut, "w2")}
}

func TestWaitUntilHoldsTheTokenUntilTheClockTime(t *testing.T) {
	e := newWfEnv() // it is 10:00 in Vietnam
	nodes, edges := waitUntilGraph()
	var shown *time.Time
	e.onNap = func(int) {
		if s, ok := e.stateOf("u"); ok && s.Status == domain.NodeWaiting && shown == nil {
			shown = s.WaitUntil
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k0", "k1"), ReadyAt: e.clock.Add(time.Minute)})

	want := time.Date(2026, 11, 21, 9, 0, 0, 0, vnZone)
	if shown == nil || !shown.Equal(want) {
		t.Fatalf("the block shows when it will go on: %v, want %v", shown, want)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if end := e.t0.Add(e.elapsed()); end.Before(want.Add(5*time.Minute)) || end.After(want.Add(10*time.Minute)) {
		t.Fatalf("the branch waited for 09:00 and then for the 5 minutes of the next block: it ended at %v", end)
	}
	if s, _ := e.stateOf("u"); s.Status != domain.NodeDone || s.Count != 2 {
		t.Fatalf("the block shows done with its 2 KOCs: %+v", s)
	}
}

func TestWaitUntilPassesTheListOnUntouchedAndCanBeStopped(t *testing.T) {
	e := newWfEnv()
	nodes, edges := waitUntilGraph()
	e.onNap = func(n int) {
		if n == 30 {
			e.store.askToStop()
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: sentKocs(e.clock, "k0", "k1"), ReadyAt: e.clock.Add(time.Minute)})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "stopped_by_user" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if e.elapsed() > time.Hour {
		t.Fatalf("the stop ends the wait at once, not after %v", e.elapsed())
	}
	if _, entered := e.stateOf("w2"); entered {
		t.Fatal("the next block never started")
	}
}
