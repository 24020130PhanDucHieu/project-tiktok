package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeStages is the KOC service as the stage block uses it: finding an active KOC and applying a stage update.
type fakeStages struct {
	kocs       map[string]*domain.Koc
	blocked    map[string]bool // ApplyStage refuses these (do not contact), as the real one does
	applied    map[string]string
	actions    []string
	failApply  int // the first N ApplyStage calls fail
	applyCalls int
}

func newFakeStages() *fakeStages {
	return &fakeStages{kocs: map[string]*domain.Koc{}, blocked: map[string]bool{}, applied: map[string]string{}}
}

func (f *fakeStages) add(id string) {
	f.kocs[id] = &domain.Koc{ID: domain.NewID(), TiktokID: id, Status: domain.KocActive}
}

func (f *fakeStages) FindActive(_ context.Context, id string) (*domain.Koc, error) {
	if k := f.kocs[id]; k != nil {
		return k, nil
	}
	return nil, domain.NotFound("Không tìm thấy KOC đang hoạt động")
}

func (f *fakeStages) ApplyStage(_ context.Context, koc *domain.Koc, p StageUpdate) (*StageResult, error) {
	f.applyCalls++
	if f.failApply > 0 {
		f.failApply--
		return nil, errors.New("db down")
	}
	if f.blocked[koc.TiktokID] {
		return nil, domain.Conflict("KOC này đang bị chặn liên hệ (da_hop_tac = true)")
	}
	f.actions = append(f.actions, p.Action)
	f.applied[koc.TiktokID] = *p.Stage
	return &StageResult{Koc: koc}, nil
}

func stageGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), {ID: "s", Type: domain.NodeStage, Stage: domain.StageNoReply}, wwait("w", 5)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "s"), wedge("s", domain.PortOut, "w")}
}

func TestStageBlockMovesEveryKocAndSkipsTheOnesThatMayNotBeTouched(t *testing.T) {
	e := newWfEnv()
	f := newFakeStages()
	f.add("k0")
	f.add("k1")
	f.blocked["k1"] = true // already collaborated: blocked from contact
	// k2 is gone
	e.r.WithStages(f)
	nodes, edges := stageGraph()
	var carried []domain.TokenKoc
	var seen bool
	e.watchTokenAt("w", &carried, &seen)
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0", "k1", "k2"), ReadyAt: e.clock})

	if len(f.applied) != 1 || f.applied["k0"] != domain.StageNoReply {
		t.Fatalf("only the KOC that may be changed is moved, to the stage of the block: %+v", f.applied)
	}
	for _, a := range f.actions {
		if a != "set_stage" {
			t.Fatalf("the block only ever sets a stage: %q", a)
		}
	}
	if ids(carried) != "k0,k1,k2" {
		t.Fatalf("a stage block does not change the list: %q", ids(carried))
	}
	s, _ := e.stateOf("s")
	if s.Status != domain.NodeDone || s.Count != 3 || !strings.Contains(s.Reason, "1 đã đổi") || !strings.Contains(s.Reason, "2 bỏ qua") {
		t.Fatalf("the block says what it did: %+v", s)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestStageBlockIsSafeToRunTwice(t *testing.T) {
	e := newWfEnv()
	f := newFakeStages()
	f.add("k0")
	e.r.WithStages(f)
	nodes, edges := stageGraph()
	tok := domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0"), ReadyAt: e.clock}
	wf := e.begin(nodes, edges, tok)
	e.r.advanceStage(context.Background(), wf, nodes[1], tok) // the server was restarted in the middle of the block
	if f.applyCalls < 2 || f.applied["k0"] != domain.StageNoReply {
		t.Fatalf("it ran twice and the stage is the same: %d calls, %+v", f.applyCalls, f.applied)
	}
}

func TestStageBlockReadsAgainWhenTheStoreBlinksAndGivesUpAfterwards(t *testing.T) {
	e := newWfEnv()
	f := newFakeStages()
	f.add("k0")
	f.failApply = 2
	e.r.WithStages(f)
	nodes, edges := stageGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if f.applied["k0"] != domain.StageNoReply || e.elapsed() < 2*time.Minute {
		t.Fatalf("moved after the store came back, a minute after each failure: %+v after %v", f.applied, e.elapsed())
	}

	e2 := newWfEnv()
	f2 := newFakeStages()
	f2.add("k0")
	f2.failApply = 1000
	e2.r.WithStages(f2)
	e2.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0"), ReadyAt: e2.clock})
	if got := e2.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("a store that keeps failing does not block the workflow for ever: %s %q", got.Status, got.StopReason)
	}
	if s, _ := e2.stateOf("s"); !strings.Contains(s.Reason, "1 lỗi") {
		t.Fatalf("the block says one failed: %+v", s)
	}
}

func TestAStageBlockWithoutTheKocServiceStopsTheWorkflow(t *testing.T) {
	e := newWfEnv()
	nodes, edges := stageGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if got := e.store.only(); got.Status != domain.WorkflowStopped || got.StopReason != "stage_unavailable" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}
