package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memWorkflows keeps workflows in memory like the repository does: it stores copies, so what the runner changes is lost
// unless it is written through the store.
type memWorkflows struct {
	mu    sync.Mutex
	clock func() time.Time
	wfs   []*domain.Workflow
}

func cloneWorkflow(w *domain.Workflow) *domain.Workflow {
	cp := *w
	cp.Tokens = make([]domain.WorkflowToken, len(w.Tokens))
	for i, t := range w.Tokens {
		t.Kocs = append([]domain.TokenKoc(nil), t.Kocs...)
		t.Done = append([]domain.TokenKoc(nil), t.Done...)
		cp.Tokens[i] = t
	}
	cp.States = append([]domain.NodeState(nil), w.States...)
	return &cp
}

func (m *memWorkflows) find(id domain.ID) *domain.Workflow {
	for _, w := range m.wfs {
		if w.ID == id {
			return w
		}
	}
	return nil
}

func (m *memWorkflows) Create(_ context.Context, wf *domain.Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.wfs {
		if w.Status == domain.WorkflowRunning {
			return ErrWorkflowInProgress
		}
	}
	wf.ID, wf.CreatedAt, wf.Status = domain.NewID(), m.clock(), domain.WorkflowRunning
	m.wfs = append(m.wfs, cloneWorkflow(wf))
	return nil
}
func (m *memWorkflows) Running(context.Context) (*domain.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.wfs {
		if w.Status == domain.WorkflowRunning {
			return cloneWorkflow(w), nil
		}
	}
	return nil, nil
}
func (m *memWorkflows) SaveTokens(_ context.Context, id domain.ID, tokens []domain.WorkflowToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := cloneWorkflow(&domain.Workflow{Tokens: tokens})
	m.find(id).Tokens = cp.Tokens
	return nil
}
func (m *memWorkflows) SetState(_ context.Context, id domain.ID, st domain.NodeState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.find(id)
	for i := range w.States {
		if w.States[i].NodeID == st.NodeID {
			w.States[i] = st
			return nil
		}
	}
	w.States = append(w.States, st)
	return nil
}
func (m *memWorkflows) AddEvent(_ context.Context, id domain.ID, ev domain.WorkflowEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.find(id)
	w.Log = append(w.Log, ev)
	return nil
}
func (m *memWorkflows) StopRequested(_ context.Context, id domain.ID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.find(id).StopRequested, nil
}
func (m *memWorkflows) Finish(_ context.Context, id domain.ID, status, reason string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.find(id)
	w.Status, w.StopReason, w.FinishedAt, w.Tokens = status, reason, &at, nil
	return nil
}
func (m *memWorkflows) only() *domain.Workflow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneWorkflow(m.wfs[0])
}
func (m *memWorkflows) askToStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wfs[0].StopRequested = true
}

// wfEnv is a runner on a clock that only moves when the runner sleeps.
type wfEnv struct {
	r       *WorkflowRunner
	imports *fakeWfImports
	runs    *fakeWfRuns
	replies *fakeReplies
	sw      *memSwitch
	store   *memWorkflows
	clock   time.Time
	t0      time.Time
	naps    int
	onNap   func(n int) // called at every tick, after the clock moved
}

func newWfEnv() *wfEnv {
	e := &wfEnv{clock: time.Date(2026, 11, 20, 10, 0, 0, 0, vnZone)}
	e.t0 = e.clock
	e.store = &memWorkflows{clock: func() time.Time { return e.clock }}
	e.imports = &fakeWfImports{}
	e.sw = &memSwitch{}
	e.runs = &fakeWfRuns{clock: func() time.Time { return e.clock }, sw: e.sw, script: map[string]string{}, sentOn: map[string]int{}}
	e.replies = &fakeReplies{answers: map[string]replyAnswer{}}
	e.r = NewWorkflowRunner(e.store, e.imports, e.runs, e.sw, e.replies)
	e.r.now = func() time.Time { return e.clock }
	e.r.tick = time.Minute
	e.r.sleep = func(_ context.Context, d time.Duration) error {
		e.clock = e.clock.Add(d)
		e.naps++
		if e.onNap != nil {
			e.onNap(e.naps)
		}
		return nil
	}
	return e
}

func (e *wfEnv) elapsed() time.Duration { return e.clock.Sub(e.t0) }

func (e *wfEnv) stateOf(nodeID string) (domain.NodeState, bool) {
	for _, s := range e.store.only().States {
		if s.NodeID == nodeID {
			return s, true
		}
	}
	return domain.NodeState{}, false
}

func wnode(id, typ string) domain.WorkflowNode { return domain.WorkflowNode{ID: id, Type: typ} }
func wwait(id string, minutes int) domain.WorkflowNode {
	return domain.WorkflowNode{ID: id, Type: domain.NodeWait, WaitMinutes: minutes}
}
func wedge(from, port, to string) domain.WorkflowEdge {
	return domain.WorkflowEdge{ID: from + ">" + to, From: from, Port: port, To: to}
}

func tokenKocs(ids ...string) []domain.TokenKoc {
	out := make([]domain.TokenKoc, len(ids))
	for i, id := range ids {
		out[i] = domain.TokenKoc{KocID: id}
	}
	return out
}

// begin starts a workflow with tokens put where the test wants them (the import block is not under test here).
func (e *wfEnv) begin(nodes []domain.WorkflowNode, edges []domain.WorkflowEdge, tokens ...domain.WorkflowToken) *domain.Workflow {
	wf := &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, By: "a@x.vn", Source: "TikToday"}
	started, err := e.r.begin(context.Background(), wf, tokens)
	if err != nil {
		panic(err)
	}
	e.r.Wait()
	return started
}

func TestWaitHoldsTheTokenUntilItsTimeAndThenMovesOn(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 30), wwait("w2", 10)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w"), wedge("w", domain.PortOut, "w2")}
	var at []string // the block the only token is at, at every tick
	var when []time.Duration
	e.onNap = func(int) {
		toks := e.store.only().Tokens
		if len(toks) == 1 {
			at, when = append(at, toks[0].NodeID), append(when, e.elapsed())
			if len(toks[0].Kocs) != 2 || toks[0].Kocs[0].KocID != "k1" || toks[0].Kocs[1].KocID != "k2" {
				t.Errorf("the list of KOCs travels untouched: %+v", toks[0].Kocs)
			}
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k1", "k2"), ReadyAt: e.clock.Add(30 * time.Minute)})

	sawSecond := false
	for i, node := range at {
		if node == "w2" {
			sawSecond = true
			if when[i] < 30*time.Minute {
				t.Fatalf("the token left the 30 minute wait after %v", when[i])
			}
		}
		if node == "w" && when[i] >= 31*time.Minute {
			t.Fatalf("the token is still at the first wait after %v", when[i])
		}
	}
	if !sawSecond {
		t.Fatal("the token reached the second wait")
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone || e.elapsed() < 40*time.Minute {
		t.Fatalf("done only after both waits: %s after %v", got.Status, e.elapsed())
	}
	if s, _ := e.stateOf("w"); s.Status != domain.NodeDone || s.Count != 2 {
		t.Fatalf("the first wait shows done with its 2 KOCs: %+v", s)
	}
	if s, _ := e.stateOf("w2"); s.Status != domain.NodeDone {
		t.Fatalf("the second wait is done too: %+v", s)
	}
}

func TestAWaitThatIsEnteredShowsItsEndTimeOnTheScreen(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 30), wwait("w2", 90)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w"), wedge("w", domain.PortOut, "w2")}
	var seen *time.Time
	e.onNap = func(int) {
		if s, ok := e.stateOf("w2"); ok && s.Status == domain.NodeWaiting && seen == nil {
			seen = s.WaitUntil
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k1"), ReadyAt: e.clock.Add(30 * time.Minute)})
	if seen == nil {
		t.Fatal("a block that waits shows when its wait ends")
	}
	if want := e.t0.Add(30*time.Minute + 90*time.Minute); seen.Before(want.Add(-time.Minute)) || seen.After(want.Add(time.Minute)) {
		t.Fatalf("the second wait ends 90 minutes after the first: %v, want about %v", seen, want)
	}
}

func TestTheWorkflowIsDoneWhenNoTokenIsLeft(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 5)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w")}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k1"), ReadyAt: e.clock.Add(5 * time.Minute)})
	got := e.store.only()
	if got.Status != domain.WorkflowDone || got.StopReason != "" || len(got.Tokens) != 0 || got.FinishedAt == nil {
		t.Fatalf("done with no token left: %+v", got)
	}
}

func TestStopEndsTheWorkflowAndKeepsTheNextBlockFromRunning(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 600), wwait("w2", 10)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w"), wedge("w", domain.PortOut, "w2")}
	e.onNap = func(n int) {
		if n == 3 {
			e.store.askToStop()
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k1"), ReadyAt: e.clock.Add(600 * time.Minute)})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "stopped_by_user" {
		t.Fatalf("stopped by the user: %s %q", got.Status, got.StopReason)
	}
	if e.elapsed() >= 10*time.Minute {
		t.Fatalf("the stop request ends the wait at once, not after %v", e.elapsed())
	}
	if s, _ := e.stateOf("w"); s.Status != domain.NodeStopped {
		t.Fatalf("the block that was waiting shows stopped: %+v", s)
	}
	if _, entered := e.stateOf("w2"); entered {
		t.Fatal("the next block never started")
	}
}

func TestAWorkflowOlderThanFourteenDaysExpires(t *testing.T) {
	e := newWfEnv()
	e.r.tick = time.Hour
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", domain.MaxWaitMinutes), wwait("w2", 60)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w"), wedge("w", domain.PortOut, "w2")}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k1"), ReadyAt: e.clock.Add(domain.MaxWaitMinutes * time.Minute)})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "expired" {
		t.Fatalf("expired: %s %q", got.Status, got.StopReason)
	}
	if e.elapsed() > domain.MaxWorkflowAge+2*time.Hour {
		t.Fatalf("it expires right after the fourteen days, not after %v", e.elapsed())
	}
}

func TestTwoBranchesWaitAtTheSameTimeWithoutBlockingEachOther(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wwait("wA", 60), wwait("wB", 10), wwait("wC", 5)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortReplied, "wA"), wedge("q", domain.PortUnanswered, "wB"), wedge("wB", domain.PortOut, "wC")}
	var cEntered time.Duration
	var aStillWaiting bool
	e.onNap = func(int) {
		toks := e.store.only().Tokens
		for _, tk := range toks {
			if tk.NodeID == "wC" && cEntered == 0 {
				cEntered = e.elapsed()
				for _, other := range toks {
					if other.NodeID == "wA" {
						aStillWaiting = true
					}
				}
			}
		}
	}
	e.begin(nodes, edges,
		domain.WorkflowToken{NodeID: "wA", Kocs: tokenKocs("a1"), ReadyAt: e.clock.Add(60 * time.Minute)},
		domain.WorkflowToken{NodeID: "wB", Kocs: tokenKocs("b1"), ReadyAt: e.clock.Add(10 * time.Minute)})
	if cEntered == 0 || cEntered > 12*time.Minute {
		t.Fatalf("the short branch moved on after about 10 minutes, not %v: the long one must not hold it", cEntered)
	}
	if !aStillWaiting {
		t.Fatal("the long branch was still waiting while the short one moved on")
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone || e.elapsed() < 60*time.Minute {
		t.Fatalf("done only when the long branch is done too: %s after %v", got.Status, e.elapsed())
	}
}

func TestStartRefusesWhatCannotRun(t *testing.T) {
	e := newWfEnv()
	ctx := context.Background()
	rows := func(n int) []domain.ImportRow {
		out := make([]domain.ImportRow, n)
		for i := range out {
			out[i] = domain.ImportRow{Line: i + 2, TiktokID: "k", Status: domain.RowQueued}
		}
		return out
	}
	good := []domain.WorkflowNode{wnode("i", domain.NodeImport)}
	if _, err := e.r.Start(ctx, "a", &domain.Workflow{Nodes: []domain.WorkflowNode{wwait("w", 5)}, Rows: rows(1)}); err == nil {
		t.Fatal("a graph with no import block is refused")
	}
	if _, err := e.r.Start(ctx, "a", &domain.Workflow{Nodes: good}); err == nil {
		t.Fatal("no row is refused")
	}
	if _, err := e.r.Start(ctx, "a", &domain.Workflow{Nodes: good, Rows: rows(domain.MaxWorkflowRows + 1)}); err == nil {
		t.Fatal("more than 200 rows is refused")
	}
	invalid := rows(2)
	invalid[0].Status, invalid[1].Status = domain.RowInvalid, domain.RowInvalid
	if _, err := e.r.Start(ctx, "a", &domain.Workflow{Nodes: good, Rows: invalid}); err == nil {
		t.Fatal("a sheet where every row was refused is refused")
	}
	if len(e.store.wfs) != 0 {
		t.Fatal("nothing was stored for a refused workflow")
	}
}
