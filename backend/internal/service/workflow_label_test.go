package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeLabels is the KOC store and the conversation store as the label block reads and writes them. AddLabel keeps a set, like
// $addToSet does.
type fakeLabels struct {
	kocs     map[string]*domain.Koc          // by id; a missing or archived KOC is simply absent
	convs    map[string]*domain.Conversation // by handle
	added    map[domain.ID][]string
	addCalls int
	failAdds int // the first N AddLabel calls fail
}

func newFakeLabels() *fakeLabels {
	return &fakeLabels{kocs: map[string]*domain.Koc{}, convs: map[string]*domain.Conversation{}, added: map[domain.ID][]string{}}
}

// koc adds a KOC to the store, with a conversation when hasThread.
func (f *fakeLabels) koc(id, tiktokID string, hasThread bool) *domain.Conversation {
	f.kocs[id] = &domain.Koc{ID: domain.NewID(), TiktokID: tiktokID, Status: domain.KocActive}
	if !hasThread {
		return nil
	}
	c := &domain.Conversation{ID: domain.NewID()}
	f.convs["@"+tiktokID] = c
	return c
}

func (f *fakeLabels) FindActiveByID(_ context.Context, id string) (*domain.Koc, error) {
	return f.kocs[id], nil
}
func (f *fakeLabels) FindByHandle(_ context.Context, handle string) (*domain.Conversation, error) {
	return f.convs[handle], nil
}
func (f *fakeLabels) AddLabel(_ context.Context, convID domain.ID, labelID string) error {
	f.addCalls++
	if f.failAdds > 0 {
		f.failAdds--
		return errors.New("db down")
	}
	for _, l := range f.added[convID] {
		if l == labelID {
			return nil
		}
	}
	f.added[convID] = append(f.added[convID], labelID)
	return nil
}

const testLabel = "65a1b2c3d4e5f60718293a4b"

func labelGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), {ID: "l", Type: domain.NodeLabel, LabelID: testLabel}, wwait("w", 5)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "l"), wedge("l", domain.PortOut, "w")}
}

func TestLabelBlockLabelsTheConversationOfEveryKocThatHasOneAndPassesAllOn(t *testing.T) {
	e := newWfEnv()
	f := newFakeLabels()
	c0 := f.koc("k0", "a", true)
	f.koc("k1", "b", false) // no conversation
	// k2 is not in the store at all (archived or deleted)
	e.r.WithLabeler(f)
	nodes, edges := labelGraph()
	var carried []domain.TokenKoc
	var seen bool
	e.watchTokenAt("w", &carried, &seen)
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "l", Kocs: tokenKocs("k0", "k1", "k2"), ReadyAt: e.clock})

	if len(f.added) != 1 || len(f.added[c0.ID]) != 1 || f.added[c0.ID][0] != testLabel {
		t.Fatalf("only the KOC that has a conversation gets the label: %+v", f.added)
	}
	if ids(carried) != "k0,k1,k2" {
		t.Fatalf("a label block does not change the list: %q", ids(carried))
	}
	s, _ := e.stateOf("l")
	if s.Status != domain.NodeDone || s.Count != 3 || !strings.Contains(s.Reason, "1 đã gắn nhãn") || !strings.Contains(s.Reason, "2 không có cuộc trò chuyện") {
		t.Fatalf("the block says what it did: %+v", s)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestLabelBlockIsSafeToRunTwice(t *testing.T) {
	e := newWfEnv()
	f := newFakeLabels()
	c := f.koc("k0", "a", true)
	e.r.WithLabeler(f)
	nodes, edges := labelGraph()
	tok := domain.WorkflowToken{NodeID: "l", Kocs: tokenKocs("k0"), ReadyAt: e.clock}
	wf := e.begin(nodes, edges, tok)
	// the server was restarted in the middle of the block: it runs again over the same KOCs
	e.r.advanceLabel(context.Background(), wf, nodes[1], tok)
	if f.addCalls < 2 || len(f.added[c.ID]) != 1 {
		t.Fatalf("the block ran twice (%d calls) and the label is on the conversation once: %+v", f.addCalls, f.added)
	}
}

func TestLabelBlockReadsAgainWhenTheStoreBlinks(t *testing.T) {
	e := newWfEnv()
	f := newFakeLabels()
	c := f.koc("k0", "a", true)
	f.failAdds = 2
	e.r.WithLabeler(f)
	nodes, edges := labelGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "l", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if len(f.added[c.ID]) != 1 {
		t.Fatalf("the label was put on after the store came back: %+v", f.added)
	}
	if e.elapsed() < 2*time.Minute {
		t.Fatalf("it waited a minute after each failure: %v", e.elapsed())
	}
}

func TestLabelBlockGivesUpOnAKocThatKeepsFailingAndGoesOn(t *testing.T) {
	e := newWfEnv()
	f := newFakeLabels()
	f.koc("k0", "a", true)
	f.failAdds = 1000
	e.r.WithLabeler(f)
	nodes, edges := labelGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "l", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("a store that keeps failing does not block the workflow for ever: %s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("l"); !strings.Contains(s.Reason, "1 lỗi") {
		t.Fatalf("the block says one failed: %+v", s)
	}
}

func TestALabelBlockWithoutTheLabelStoreStopsTheWorkflowInsteadOfDoingNothing(t *testing.T) {
	e := newWfEnv()
	nodes, edges := labelGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "l", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "label_unavailable" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}
