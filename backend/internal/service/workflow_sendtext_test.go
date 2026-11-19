package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func sendTextNode(id string) domain.WorkflowNode {
	return domain.WorkflowNode{ID: id, Type: domain.NodeSendText, Text: "Chào {ten}, anh chị xem giúp em nhé"}
}

func sendTextGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), sendTextNode("t")}, []domain.WorkflowEdge{wedge("i", domain.PortOut, "t")}
}

func TestSendTextSendsOneTextStepThroughTheCampaignRunner(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendTextGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(3), ReadyAt: e.clock})
	if len(e.runs.runs) != 1 {
		t.Fatalf("one run: %d", len(e.runs.runs))
	}
	run := e.runs.runs[0]
	if len(run.Steps) != 1 || run.Steps[0].Kind != domain.StepText || run.Steps[0].Text != "Chào {ten}, anh chị xem giúp em nhé" {
		t.Fatalf("the text is the only step: %+v", run.Steps)
	}
	if run.CampaignName != "Tin lẻ" || len(run.Targets) != 3 || run.NodeID != "t#1" {
		t.Fatalf("%q %d targets, tag %q", run.CampaignName, len(run.Targets), run.NodeID)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("t"); s.Status != domain.NodeDone || s.Count != 3 {
		t.Fatalf("%+v", s)
	}
}

func TestSendTextKeepsTheRulesOfTheSendBlock(t *testing.T) {
	nodes, edges := sendTextGraph()

	t.Run("chunks of fifty", func(t *testing.T) {
		e := newWfEnv()
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(120), ReadyAt: e.clock})
		var sizes []int
		for _, s := range e.runs.starts {
			sizes = append(sizes, s.size)
		}
		if len(sizes) != 3 || sizes[0] != 50 || sizes[1] != 50 || sizes[2] != 20 {
			t.Fatalf("%v", sizes)
		}
	})

	t.Run("the window", func(t *testing.T) {
		e := newWfEnv()
		e.clock = time.Date(2026, 11, 20, 22, 0, 0, 0, vnZone)
		e.t0 = e.clock
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(2), ReadyAt: e.clock})
		open := time.Date(2026, 11, 21, 8, 30, 0, 0, vnZone)
		if at := e.startsAt(); len(at) != 1 || at[0].Before(open) || at[0].After(open.Add(3*time.Minute)) {
			t.Fatalf("%v", at)
		}
	})

	t.Run("the daily limit goes on tomorrow", func(t *testing.T) {
		e := newWfEnv()
		e.runs.dailyLimit = 3
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(5), ReadyAt: e.clock})
		if len(e.runs.starts) != 2 || e.runs.starts[1].size != 2 {
			t.Fatalf("%+v", e.runs.starts)
		}
	})

	t.Run("a KOC who answered is not messaged", func(t *testing.T) {
		e := newWfEnv()
		e.replies.answer("k1", true, true)
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: sentKocs(e.clock.Add(-26*time.Hour), "k0", "k1"), ReadyAt: e.clock})
		if len(e.runs.starts) != 1 || e.runs.starts[0].size != 1 || e.runs.runs[0].Targets[0].KocID != "k0" {
			t.Fatalf("%+v", e.runs.starts)
		}
	})

	t.Run("stopping turns the switch off", func(t *testing.T) {
		e := newWfEnv()
		e.onNap = func(n int) {
			if n == 3 {
				e.store.askToStop()
			}
		}
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(5), ReadyAt: e.clock})
		if got := e.store.only(); got.Status != domain.WorkflowStopped || e.sw.on {
			t.Fatalf("%s switch on=%v", got.Status, e.sw.on)
		}
	})

	t.Run("a restart does not start the chunk twice", func(t *testing.T) {
		e := newWfEnv()
		run := e.addRun("t#1", domain.RunRunning, "k0", "k1")
		_ = run
		e.sw.on = true
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(2), Chunk: 1, ReadyAt: e.clock})
		if len(e.runs.starts) != 0 {
			t.Fatalf("the run of that chunk is followed: %d starts", len(e.runs.starts))
		}
	})
}

func TestTwoHundredKocsThroughASendTextBlockFinishWithinFourteenDays(t *testing.T) {
	e := newWfEnv()
	e.runs.dailyLimit = 30
	nodes, edges := sendTextGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "t", Kocs: kocTokens(200), ReadyAt: e.clock})
	if got := e.store.only(); got.Status != domain.WorkflowDone || e.elapsed() > 8*24*time.Hour {
		t.Fatalf("%s %q after %v", got.Status, got.StopReason, e.elapsed())
	}
	seen := map[string]int{}
	for _, r := range e.runs.runs {
		for _, tg := range r.Targets {
			if tg.Status == domain.TargetSent {
				seen[tg.KocID]++
			}
		}
	}
	if len(seen) != 200 {
		t.Fatalf("%d KOCs were sent to", len(seen))
	}
	for i := 0; i < 200; i++ {
		if seen["k"+strconv.Itoa(i)] != 1 {
			t.Fatalf("KOC k%d was sent to %d times", i, seen["k"+strconv.Itoa(i)])
		}
	}
}
