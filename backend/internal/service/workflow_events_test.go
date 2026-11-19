package service

import (
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// eventKocs is the KOCs of the events of one block and kind, joined, in the order they were written.
func eventKocs(wf *domain.Workflow, nodeID, kind string) string {
	var out []string
	for _, ev := range wf.Log {
		if ev.NodeID == nodeID && ev.Kind == kind {
			out = append(out, strings.Join(ev.Kocs, ","))
		}
	}
	return strings.Join(out, ";")
}

func TestTheLogSaysWhichKocsEnteredChangedWereSkippedAndLeftEveryBlock(t *testing.T) {
	e := newWfEnv()
	f := newFakeStages()
	f.add("k0")
	f.add("k1")
	f.blocked["k1"] = true
	e.r.WithStages(f)
	nodes, edges := stageGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0", "k1", "k2"), ReadyAt: e.clock})

	wf := e.store.only()
	if got := eventKocs(wf, "s", domain.EventIn); got != "k0,k1,k2" {
		t.Fatalf("who came to the stage block: %q", got)
	}
	if got := eventKocs(wf, "s", domain.EventDid); got != "k0" {
		t.Fatalf("who was changed: %q", got)
	}
	if got := eventKocs(wf, "s", domain.EventSkip); got != "k1,k2" {
		t.Fatalf("who was skipped: %q", got)
	}
	if got := eventKocs(wf, "s", domain.EventOut); got != "k0,k1,k2" {
		t.Fatalf("who left the block: %q", got)
	}
	if got := eventKocs(wf, "w", domain.EventIn); got != "k0,k1,k2" {
		t.Fatalf("who came to the next block: %q", got)
	}
	for _, ev := range wf.Log {
		if ev.At.IsZero() {
			t.Fatalf("every event has the moment it happened: %+v", ev)
		}
	}
}
