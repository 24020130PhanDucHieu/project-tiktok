package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeSearcher is the directory: it answers a page of the KOCs of one stage and folder, newest first, like the repository.
type fakeSearcher struct {
	kocs  []domain.Koc
	calls int
	err   error
}

func (f *fakeSearcher) Search(_ context.Context, q domain.KocQuery, skip, limit int64) ([]domain.Koc, int64, error) {
	f.calls++
	if f.err != nil {
		return nil, 0, f.err
	}
	var match []domain.Koc
	for _, k := range f.kocs {
		if (q.Stage == "" || k.Stage == q.Stage) && (q.Category == "" || k.Category == q.Category) {
			match = append(match, k)
		}
	}
	total := int64(len(match))
	if skip >= total {
		return nil, total, nil
	}
	end := skip + limit
	if end > total {
		end = total
	}
	return match[skip:end], total, nil
}

func pickKoc(n int, stage, folder string, over func(*domain.Koc)) domain.Koc {
	k := domain.Koc{ID: domain.NewID(), TiktokID: "k" + strconv.Itoa(n), Stage: stage, Category: folder, Status: domain.KocActive}
	if over != nil {
		over(&k)
	}
	return k
}

func pickIDs(ts []domain.TokenKoc) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.KocID)
	}
	return out
}

func TestPickLeavesOutBlockedKocsAndKeepsTheOrder(t *testing.T) {
	a, b, c := pickKoc(1, "SHIPPING", "", nil), pickKoc(2, "SHIPPING", "", func(k *domain.Koc) { k.Dnc = true }), pickKoc(3, "SHIPPING", "", func(k *domain.Koc) { k.DaHopTac = true })
	d := pickKoc(4, "SHIPPING", "", nil)
	p := NewKocPicker(&fakeSearcher{kocs: []domain.Koc{a, b, c, d}})
	got, err := p.Pick(context.Background(), []string{"SHIPPING"}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pickIDs(got); len(ids) != 2 || ids[0] != a.ID.Hex() || ids[1] != d.ID.Hex() {
		t.Fatalf("the KOCs that must not be contacted are left out, the others keep the directory's order: %v", ids)
	}
	if got[0].SentAt != nil {
		t.Fatal("a picked KOC has not been sent to by this workflow")
	}
}

func TestPickStopsAtTheLimit(t *testing.T) {
	var all []domain.Koc
	for i := 0; i < 30; i++ {
		all = append(all, pickKoc(i, "REPLIED", "", nil))
	}
	got, err := NewKocPicker(&fakeSearcher{kocs: all}).Pick(context.Background(), []string{"REPLIED"}, "", 12)
	if err != nil || len(got) != 12 {
		t.Fatalf("12 KOCs: %d %v", len(got), err)
	}
}

func TestPickMergesSeveralStagesWithoutRepeatingAKoc(t *testing.T) {
	a, b := pickKoc(1, "SHIPPING", "", nil), pickKoc(2, "DELIVERED", "", nil)
	// a KOC cannot be in two stages at once, but the same KOC may be found twice if it changed stage while the lists were read
	f := &fakeSearcher{kocs: []domain.Koc{a, b, a}}
	got, err := NewKocPicker(f).Pick(context.Background(), []string{"SHIPPING", "DELIVERED"}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pickIDs(got); len(ids) != 2 || ids[0] != a.ID.Hex() || ids[1] != b.ID.Hex() {
		t.Fatalf("both stages, each KOC once: %v", ids)
	}
}

func TestPickByFolderAloneAndByFolderAndStage(t *testing.T) {
	a, b, c := pickKoc(1, "SHIPPING", "Làm đẹp", nil), pickKoc(2, "SHIPPING", "Gia dụng", nil), pickKoc(3, "REPLIED", "Làm đẹp", nil)
	p := NewKocPicker(&fakeSearcher{kocs: []domain.Koc{a, b, c}})
	byFolder, _ := p.Pick(context.Background(), nil, "Làm đẹp", 50)
	if ids := pickIDs(byFolder); len(ids) != 2 || ids[0] != a.ID.Hex() || ids[1] != c.ID.Hex() {
		t.Fatalf("a folder with no stage: %v", ids)
	}
	both, _ := p.Pick(context.Background(), []string{"SHIPPING"}, "Làm đẹp", 50)
	if ids := pickIDs(both); len(ids) != 1 || ids[0] != a.ID.Hex() {
		t.Fatalf("a folder and a stage: %v", ids)
	}
}

func TestPickReadsPastAPageFullOfBlockedKocs(t *testing.T) {
	var all []domain.Koc
	for i := 0; i < 130; i++ { // 130 KOCs that must not be contacted come first
		all = append(all, pickKoc(i, "SHIPPING", "", func(k *domain.Koc) { k.Dnc = true }))
	}
	good := pickKoc(999, "SHIPPING", "", nil)
	all = append(all, good)
	got, err := NewKocPicker(&fakeSearcher{kocs: all}).Pick(context.Background(), []string{"SHIPPING"}, "", 5)
	if err != nil || len(got) != 1 || got[0].KocID != good.ID.Hex() {
		t.Fatalf("the one KOC after the blocked ones is found: %v %v", pickIDs(got), err)
	}
}

func TestPickReportsAFailureOfTheDirectory(t *testing.T) {
	if _, err := NewKocPicker(&fakeSearcher{err: context.DeadlineExceeded}).Pick(context.Background(), []string{"SHIPPING"}, "", 5); err == nil {
		t.Fatal("a failing directory is an error, never an empty list")
	}
}

// ---- the workflow runner starting from a pick block

type fixedPicker struct {
	kocs  []domain.TokenKoc
	calls int
}

func (f *fixedPicker) Pick(context.Context, []string, string, int) ([]domain.TokenKoc, error) {
	f.calls++
	return f.kocs, nil
}

func pickGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	p := domain.WorkflowNode{ID: "p", Type: domain.NodePick, Stages: []string{"SHIPPING"}, Limit: 50}
	return []domain.WorkflowNode{p, wwait("w", 5)}, []domain.WorkflowEdge{wedge("p", domain.PortOut, "w")}
}

func TestAWorkflowStartingFromPickNeedsNoRowsAndCarriesThePickedKocs(t *testing.T) {
	e := newWfEnv()
	picker := &fixedPicker{kocs: tokenKocs("k0", "k1", "k2")}
	e.r.WithPicker(picker)
	nodes, edges := pickGraph()
	var carried []domain.TokenKoc
	e.onNap = func(int) {
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == "w" && carried == nil {
				carried = tk.Kocs
			}
		}
	}
	if _, err := e.r.Start(context.Background(), "a@x.vn", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	e.r.Wait()
	if ids(carried) != "k0,k1,k2" {
		t.Fatalf("the picked KOCs go on in order: %q", ids(carried))
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone || len(got.Rows) != 0 {
		t.Fatalf("%s %q rows=%d", got.Status, got.StopReason, len(got.Rows))
	}
	if s, _ := e.stateOf("p"); s.Status != domain.NodeDone || s.Count != 3 {
		t.Fatalf("the pick block shows done with 3 KOCs: %+v", s)
	}
}

func TestPickWithNoMatchIsRefusedAndNothingIsStored(t *testing.T) {
	e := newWfEnv()
	e.r.WithPicker(&fixedPicker{})
	nodes, edges := pickGraph()
	if _, err := e.r.Start(context.Background(), "a@x.vn", &domain.Workflow{Nodes: nodes, Edges: edges}); err == nil {
		t.Fatal("a pick that finds nobody is refused")
	}
	if len(e.store.wfs) != 0 {
		t.Fatal("nothing was stored")
	}
}

func TestThePickedListIsFixedAtStartAndNotPickedAgainAfterARestart(t *testing.T) {
	e := newWfEnv()
	picker := &fixedPicker{kocs: tokenKocs("k0", "k1")}
	e.r.WithPicker(picker)
	nodes, edges := pickGraph()
	nodes[1] = wwait("w", 120)
	e.r.sleep = func(context.Context, time.Duration) error {
		e.naps++
		if e.naps == 1 {
			die() // the server is killed right after the start
		}
		e.clock = e.clock.Add(time.Minute)
		return nil
	}
	if _, err := e.r.Start(context.Background(), "a@x.vn", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	e.r.Wait()
	picker.kocs = tokenKocs("other") // the directory changed while the server was down
	again := NewWorkflowRunner(e.store, e.imports, e.runs, e.sw, e.replies).WithPicker(picker)
	again.now, again.tick = e.r.now, e.r.tick
	again.sleep = func(_ context.Context, d time.Duration) error { e.clock = e.clock.Add(d); return nil }
	again.Resume(context.Background())
	again.Wait()
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("the workflow went on after the restart: %s %q", got.Status, got.StopReason)
	}
	if picker.calls != 1 {
		t.Fatalf("the list is taken once, when the workflow starts: %d", picker.calls)
	}
}

func TestAWorkflowStartingFromPickWithoutAPickerIsRefused(t *testing.T) {
	e := newWfEnv()
	nodes, edges := pickGraph()
	if _, err := e.r.Start(context.Background(), "a@x.vn", &domain.Workflow{Nodes: nodes, Edges: edges}); err == nil {
		t.Fatal("a runner that cannot pick must refuse a pick block rather than start with nobody")
	}
}
