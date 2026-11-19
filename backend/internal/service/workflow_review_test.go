package service

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Tests for the findings of the final review of the workflow branch.

func (e *wfEnv) addRun(nodeID, status string, kocs ...string) *domain.BulkRun {
	r := &domain.BulkRun{ID: domain.NewID(), NodeID: nodeID, Status: status}
	for _, k := range kocs {
		r.Targets = append(r.Targets, domain.RunTarget{KocID: k, Status: domain.TargetQueued})
	}
	e.runs.runs = append(e.runs.runs, r)
	e.runs.ignoreWorkflows = true
	return r
}

func TestStoppingTurnsTheSwitchOffForARunThatWasStartedButNeverRecorded(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	wf := &domain.Workflow{ID: domain.NewID(), Nodes: nodes, Edges: edges}
	e.addRun("s#1", domain.RunRunning, "k0", "k1") // the run was started, the server died before the token was saved
	e.sw.on = true
	wf.Tokens = []domain.WorkflowToken{{NodeID: "s", Kocs: kocTokens(2), Chunk: 1}} // no ChildID

	e.r.stopChildren(context.Background(), wf)

	if e.sw.on {
		t.Fatal("the run nobody wrote down goes on sending after the workflow was stopped: the switch must go off")
	}
}

func TestStoppingLeavesARunThatIsNotTheWorkflowsOwnAlone(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	wf := &domain.Workflow{ID: domain.NewID(), Nodes: nodes, Edges: edges}
	ended := e.addRun("s#1", domain.RunDone, "k0")
	e.addRun("", domain.RunRunning, "m0", "m1") // a person started a campaign by hand in the meantime
	e.sw.on = true
	wf.Tokens = []domain.WorkflowToken{{NodeID: "s", Kocs: kocTokens(1), Chunk: 1, ChildID: ended.ID.Hex()}}

	e.r.stopChildren(context.Background(), wf)

	if !e.sw.on {
		t.Fatal("the workflow's own run already ended: stopping the workflow must not kill somebody else's run")
	}
}

func TestTheKocsLeftAreFoundByIdNotByPosition(t *testing.T) {
	// A chunk of five of which k1 was dropped before the run started: the run holds k0, k2, k3, k4. The server died before the
	// token was saved, so the token still lists all five. Counting by position would put k4 back in the queue.
	for name, k1Replied := range map[string]bool{"k1 answered meanwhile": true, "k1 did not answer": false} {
		t.Run(name, func(t *testing.T) {
			e := newWfEnv()
			nodes, edges := sendGraph()
			run := e.addRun("s#1", domain.RunDone, "k0", "k2", "k3", "k4")
			for i := range run.Targets {
				run.Targets[i].Status = domain.TargetSent
			}
			e.replies.answer("k1", k1Replied, true)
			sent := e.clock.Add(-48 * time.Hour)
			e.settleEveryTick()
			e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: sentKocs(sent, "k0", "k1", "k2", "k3", "k4"), Chunk: 1, ReadyAt: e.clock})

			for _, s := range e.runs.starts {
				for _, tg := range e.runs.runs[len(e.runs.runs)-1].Targets {
					if tg.KocID == "k4" || tg.KocID == "k0" {
						t.Fatalf("%s was sent to again after the restart: %+v", tg.KocID, s)
					}
				}
			}
			wantStarts := 0
			if !k1Replied {
				wantStarts = 1
			}
			if len(e.runs.starts) != wantStarts || (wantStarts == 1 && e.runs.starts[0].size != 1) {
				t.Fatalf("k1 is the only KOC left, and only when it has not answered: %+v", e.runs.starts)
			}
		})
	}
}

func TestSendBlockSkipsAKocWhoAnsweredSinceItWasLastSent(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	e.replies.answer("k1", true, true)
	sent := e.clock.Add(-26 * time.Hour)
	toks := append(sentKocs(sent, "k0", "k1"), domain.TokenKoc{KocID: "k2"}) // k2 comes from the import: nothing to check
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: toks, ReadyAt: e.clock})

	if len(e.runs.starts) != 1 || e.runs.starts[0].size != 2 {
		t.Fatalf("the KOC who answered is not messaged: %+v", e.runs.starts)
	}
	for _, tg := range e.runs.runs[0].Targets {
		if tg.KocID == "k1" {
			t.Fatal("k1 answered after the first campaign and must not get the reminder")
		}
	}
	if s, _ := e.stateOf("s"); s.Count != 2 || s.Reason != "1 bỏ qua" {
		t.Fatalf("the block says one dropped out: %+v", s)
	}
	if calls := e.replies.calls["k2"]; len(calls) != 0 {
		t.Fatalf("a KOC that was never sent to is not asked about: %v", calls)
	}
}

func TestAKocThatCannotBeCheckedIsNotMessagedAfterTheRetries(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	e.replies.answers["k0"] = replyAnswer{errTimes: 100}
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: sentKocs(e.clock.Add(-time.Hour), "k0", "k1"), ReadyAt: e.clock})
	if len(e.runs.starts) != 1 || e.runs.starts[0].size != 1 || e.runs.runs[0].Targets[0].KocID != "k1" {
		t.Fatalf("only the KOC that could be checked is messaged: %+v", e.runs.starts)
	}
}

func TestAKocLeftForTheNextRunKeepsItsSendTime(t *testing.T) {
	e := newWfEnv()
	e.runs.dailyLimit = 1
	nodes, edges := sendGraph()
	sent := e.clock.Add(-30 * time.Hour)
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: sentKocs(sent, "k0", "k1"), ReadyAt: e.clock})
	if len(e.runs.starts) != 2 {
		t.Fatalf("one KOC a day: two runs: %+v", e.runs.starts)
	}
	for _, since := range e.replies.calls["k1"] {
		if !since.Equal(sent) {
			t.Fatalf("k1 waited a day for its turn but is still asked about the time it was last sent to (%v), not %v", sent, since)
		}
	}
	if len(e.replies.calls["k1"]) == 0 {
		t.Fatal("k1 was never checked for a reply")
	}
}
