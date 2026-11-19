package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type replyAnswer struct {
	replied, known bool
	errTimes       int // the first N calls for this KOC fail
}

// fakeReplies answers "did this KOC answer since..." from a table and remembers what it was asked.
type fakeReplies struct {
	mu      sync.Mutex
	answers map[string]replyAnswer // a KOC with no entry has not replied
	calls   map[string][]time.Time // the `since` of every call, per KOC
}

func (f *fakeReplies) RepliedSince(_ context.Context, kocID string, since time.Time) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string][]time.Time{}
	}
	f.calls[kocID] = append(f.calls[kocID], since)
	a, ok := f.answers[kocID]
	if !ok {
		return false, true, nil
	}
	if a.errTimes > 0 {
		a.errTimes--
		f.answers[kocID] = a
		return false, false, errors.New("db down")
	}
	return a.replied, a.known, nil
}

func (f *fakeReplies) answer(kocID string, replied, known bool) {
	f.answers[kocID] = replyAnswer{replied: replied, known: known}
}

func sentKocs(at time.Time, ids ...string) []domain.TokenKoc {
	out := make([]domain.TokenKoc, len(ids))
	for i, id := range ids {
		sent := at
		out[i] = domain.TokenKoc{KocID: id, SentAt: &sent}
	}
	return out
}

func splitGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wwait("wA", 60), wwait("wB", 60)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortReplied, "wA"), wedge("q", domain.PortUnanswered, "wB")}
}

// firstTokenAt records the KOCs of the first token that shows up at a block.
func (e *wfEnv) watchTokenAt(nodeID string, into *[]domain.TokenKoc, seen *bool) {
	e.onNap = func(int) {
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == nodeID && !*seen {
				*seen, *into = true, tk.Kocs
			}
		}
	}
}

func ids(ks []domain.TokenKoc) string {
	var out []string
	for _, k := range ks {
		out = append(out, k.KocID)
	}
	return strings.Join(out, ",")
}

func TestIfRepliedSplitsTheListByReply(t *testing.T) {
	e := newWfEnv()
	nodes, edges := splitGraph()
	e.replies.answer("k0", true, true)
	e.replies.answer("k2", true, true)
	sent := e.clock.Add(-48 * time.Hour)
	var a, b []domain.TokenKoc
	var seenA, seenB bool
	e.onNap = func(int) {
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == "wA" && !seenA {
				seenA, a = true, tk.Kocs
			}
			if tk.NodeID == "wB" && !seenB {
				seenB, b = true, tk.Kocs
			}
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "q", Kocs: sentKocs(sent, "k0", "k1", "k2", "k3"), ReadyAt: e.clock})
	if ids(a) != "k0,k2" || ids(b) != "k1,k3" {
		t.Fatalf("replied go one way, unanswered the other, each in order: %q and %q", ids(a), ids(b))
	}
	for _, id := range []string{"k0", "k1", "k2", "k3"} {
		if calls := e.replies.calls[id]; len(calls) != 1 || !calls[0].Equal(sent) {
			t.Fatalf("KOC %s is asked about the time its steps went out (%v): %v", id, sent, calls)
		}
	}
	s, _ := e.stateOf("q")
	if s.Status != domain.NodeDone || s.Count != 4 || !strings.Contains(s.Reason, "2 đã trả lời") || !strings.Contains(s.Reason, "2 chưa trả lời") {
		t.Fatalf("the block shows the split: %+v", s)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestAKocWhoseReplyCannotBeKnownGoesToNeitherBranch(t *testing.T) {
	e := newWfEnv()
	nodes, edges := splitGraph()
	e.replies.answer("k0", false, false) // nobody can tell (no conversation)
	e.replies.answers["k1"] = replyAnswer{errTimes: 100}
	sent := e.clock.Add(-time.Hour)
	toks := sentKocs(sent, "k0", "k1", "k3")
	toks = append(toks[:2], append([]domain.TokenKoc{{KocID: "k2"}}, toks[2:]...)...) // k2 has no send time
	var a, b []domain.TokenKoc
	var seenA, seenB bool
	e.onNap = func(int) {
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == "wA" && !seenA {
				seenA, a = true, tk.Kocs
			}
			if tk.NodeID == "wB" && !seenB {
				seenB, b = true, tk.Kocs
			}
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "q", Kocs: toks, ReadyAt: e.clock})
	if seenA || ids(b) != "k3" {
		t.Fatalf("only the KOC that is known not to have replied may be reminded: a=%q b=%q", ids(a), ids(b))
	}
	if s, _ := e.stateOf("q"); !strings.Contains(s.Reason, "3 không rõ") {
		t.Fatalf("the block says how many could not be told: %+v", s)
	}
}

func TestAFailedReadIsTriedAgainBeforeAKocIsGivenUp(t *testing.T) {
	e := newWfEnv()
	nodes, edges := splitGraph()
	e.replies.answers["k0"] = replyAnswer{replied: false, known: true, errTimes: 2} // fails twice, then answers
	var b []domain.TokenKoc
	var seenB bool
	e.watchTokenAt("wB", &b, &seenB)
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "q", Kocs: sentKocs(e.clock.Add(-time.Hour), "k0"), ReadyAt: e.clock})
	if ids(b) != "k0" {
		t.Fatalf("a read that failed twice and then worked puts the KOC in its branch: %q", ids(b))
	}
}

func TestAnUnconnectedPortEndsItsBranch(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wwait("wB", 5)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortUnanswered, "wB")}
	e.replies.answer("k0", true, true)
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "q", Kocs: sentKocs(e.clock, "k0", "k1"), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("the branch with no wire simply ends: %s %q", got.Status, got.StopReason)
	}
}

func TestBothBranchesRunIndependently(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wwait("wA", 3*24*60), wsend("sB")}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortReplied, "wA"), wedge("q", domain.PortUnanswered, "sB")}
	e.replies.answer("k0", true, true)
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "q", Kocs: sentKocs(e.clock.Add(-time.Hour), "k0", "k1"), ReadyAt: e.clock})
	if len(e.runs.starts) != 1 {
		t.Fatalf("the reminder is sent: %d runs", len(e.runs.starts))
	}
	if gap := e.runs.starts[0].at.Sub(e.t0); gap > 5*time.Minute {
		t.Fatalf("the reminder goes out at once, not after the 3 days the other branch waits: %v", gap)
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone || e.elapsed() < 3*24*time.Hour {
		t.Fatalf("done only when the long branch is done too: %s after %v", got.Status, e.elapsed())
	}
}
