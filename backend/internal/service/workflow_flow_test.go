package service

import (
	"context"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// realImports and realRuns put the real runners (with in-memory stores below them) behind the interfaces the workflow
// runner uses, the way the repositories and the runners are put together in main.go.
type realImports struct {
	*ImportRunner
	store *memImports
}

func (a realImports) FindByWorkflow(_ context.Context, workflowID, nodeID string) (*domain.ImportJob, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for i := len(a.store.jobs) - 1; i >= 0; i-- {
		if j := a.store.jobs[i]; j.WorkflowID == workflowID && j.NodeID == nodeID {
			return cloneJob(j), nil
		}
	}
	return nil, nil
}

func (a realImports) Find(_ context.Context, id string) (*domain.ImportJob, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for _, j := range a.store.jobs {
		if j.ID.Hex() == id {
			return cloneJob(j), nil
		}
	}
	return nil, domain.NotFound("không có")
}

func (a realImports) RequestStop(_ context.Context, id string) (bool, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for _, j := range a.store.jobs {
		if j.ID.Hex() == id && j.Status == domain.ImportRunning {
			j.StopRequested = true
			return true, nil
		}
	}
	return false, nil
}

type realRuns struct {
	*CampaignRunner
	store *memRuns
}

func (a realRuns) Running(ctx context.Context) (*domain.BulkRun, error) { return a.store.Running(ctx) }

func (a realRuns) FindByWorkflow(_ context.Context, workflowID, nodeID string) (*domain.BulkRun, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for i := len(a.store.runs) - 1; i >= 0; i-- {
		if r := a.store.runs[i]; r.WorkflowID == workflowID && r.NodeID == nodeID {
			return cloneRun(r), nil
		}
	}
	return nil, nil
}

func (a realRuns) Find(_ context.Context, id domain.ID) (*domain.BulkRun, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if r := a.store.find(id); r != nil {
		return cloneRun(r), nil
	}
	return nil, domain.NotFound("không có")
}

// flowEnv is a workflow runner on top of the real import runner and the real campaign runner. All three read one clock,
// which moves only when the workflow runner sleeps.
type flowEnv struct {
	we      *wfEnv
	ie      *importEnv
	re      *runEnv
	clockNs atomic.Int64
	kocs    []string // the KOCs the importer creates for k0, k1, k2
	r       *WorkflowRunner

	// the stores of the blocks of the second plan; a test that needs one sets it before it starts the workflow
	picker  KocPicker
	labels  *fakeLabels
	stages  *fakeStages
	emailer *fakeEmailer
}

func newFlowEnv() *flowEnv {
	f := &flowEnv{we: newWfEnv(), ie: newImportEnv(map[string][]string{"k3": {ImportDuplicate}}), re: newRunEnv(30)}
	f.re.sw.on = false // the workflow is the one that turns it on
	f.clockNs.Store(f.we.clock.UnixNano())
	f.re.svc.clock = func() time.Time { return time.Unix(0, f.clockNs.Load()).In(vnZone) }
	f.ie.imp.ids = map[string]domain.ID{}
	for i := 0; i < 3; i++ {
		target := f.re.addKoc("K"+strconv.Itoa(i), "097645157"+strconv.Itoa(i))
		id, _ := domain.ParseID(target.KocID)
		f.ie.imp.ids["k"+strconv.Itoa(i)] = id
		f.kocs = append(f.kocs, target.KocID)
	}
	f.r = f.runner(nil)
	return f
}

// runner builds a workflow runner over the same stores and runners; beforeSleep may stop it (see die).
func (f *flowEnv) runner(beforeSleep func(naps int)) *WorkflowRunner {
	r := NewWorkflowRunner(f.we.store, realImports{f.ie.r, f.ie.store}, realRuns{f.re.r, f.re.runs}, f.re.sw, f.we.replies)
	r.now = func() time.Time { return f.we.clock }
	r.tick = time.Minute
	if f.picker != nil {
		r.WithPicker(f.picker)
	}
	if f.labels != nil {
		r.WithLabeler(f.labels)
	}
	if f.stages != nil {
		r.WithStages(f.stages)
	}
	if f.emailer != nil {
		r.WithEmailer(f.emailer)
		r.emailPause = func() time.Duration { return 5 * time.Second }
	}
	r.sleep = func(_ context.Context, d time.Duration) error {
		f.we.clock = f.we.clock.Add(d)
		f.clockNs.Store(f.we.clock.UnixNano())
		f.we.naps++
		if beforeSleep != nil {
			beforeSleep(f.we.naps)
		}
		time.Sleep(50 * time.Microsecond) // the real runners work in their own goroutines: give them the time they need
		return nil
	}
	return r
}

// die ends the calling goroutine at once, like a server that is killed: nothing after this point runs, and in particular
// the workflow is not marked as ended.
func die() { runtime.Goexit() }

func (f *flowEnv) waitAll() {
	f.r.Wait()
	f.ie.r.Wait()
	f.re.r.Wait()
}

// restart starts a new runner over the same stores, the way main.go does after a restart.
func (f *flowEnv) restart() {
	f.r.Wait()
	f.r = f.runner(nil)
	f.r.Resume(context.Background())
	f.waitAll()
}

func (f *flowEnv) elapsed() time.Duration { return f.we.clock.Sub(f.we.t0) }

func TestAWholeWorkflowThroughTheRealRunners(t *testing.T) {
	f := newFlowEnv()
	f.we.replies.answer(f.kocs[1], true, true) // KOC 1 answers after the first campaign
	steps := twoSteps()
	send1, send2 := wsend("s1"), wsend("s2")
	send1.Steps, send2.Steps, send2.CampaignName = steps, steps, "Nhắc"
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w1", 60), send1, wwait("w2", 2*24*60), wnode("q", domain.NodeIfReplied), send2}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w1"), wedge("w1", domain.PortOut, "s1"), wedge("s1", domain.PortOut, "w2"),
		wedge("w2", domain.PortOut, "q"), wedge("q", domain.PortUnanswered, "s2")}

	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, Rows: sheetRows(4)}); err != nil {
		t.Fatal(err)
	}
	f.waitAll()

	got := f.we.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if len(f.ie.store.jobs) != 1 {
		t.Fatalf("one import job: %d", len(f.ie.store.jobs))
	}
	job := f.ie.store.only()
	if job.Rows[0].KocID != f.kocs[0] || job.Rows[3].KocID != "" || job.Rows[3].Status != domain.RowDuplicate {
		t.Fatalf("the import recorded the KOC of every added row: %+v", job.Rows)
	}
	if len(f.re.runs.runs) != 2 {
		t.Fatalf("two sends, one per campaign block: %d runs", len(f.re.runs.runs))
	}
	first, second := f.re.runs.runs[0], f.re.runs.runs[1]
	if len(first.Targets) != 3 || len(second.Targets) != 2 {
		t.Fatalf("the first campaign goes to the 3 imported KOCs, the reminder only to the 2 who did not answer: %d and %d", len(first.Targets), len(second.Targets))
	}
	if second.Targets[0].KocID != f.kocs[0] || second.Targets[1].KocID != f.kocs[2] {
		t.Fatalf("the KOC who answered is not reminded: %+v", second.Targets)
	}
	for _, run := range f.re.runs.runs {
		for _, tg := range run.Targets {
			if tg.Status != domain.TargetSent {
				t.Fatalf("every KOC of both runs was sent to: %+v", tg)
			}
		}
	}
	if len(f.re.line.calls) != (3+2)*len(steps) {
		t.Fatalf("5 KOC sends of 2 steps each went out: %d messages", len(f.re.line.calls))
	}
	if f.elapsed() < 60*time.Minute+2*24*time.Hour {
		t.Fatalf("both waits were kept: %v", f.elapsed())
	}
	if first.WorkflowID != got.ID.Hex() || second.WorkflowID != got.ID.Hex() || job.WorkflowID != got.ID.Hex() {
		t.Fatal("the three jobs are tagged with their workflow")
	}
	if on, _ := f.re.sw.Enabled(context.Background()); on {
		t.Fatal("the bulk switch is off when it is over")
	}
}

func TestRestartDuringImportDoesNotStartASecondJob(t *testing.T) {
	f := newFlowEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 5)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w")}
	f.r = f.runner(func(naps int) {
		if naps == 1 { // the job was started on the first tick
			die()
		}
	})
	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, Rows: sheetRows(4)}); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if len(f.ie.store.jobs) != 1 {
		t.Fatalf("the job the first run started is followed, not started again: %d jobs", len(f.ie.store.jobs))
	}
	if got := f.we.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestRestartDuringSendDoesNotSendTwiceToAKoc(t *testing.T) {
	f := newFlowEnv()
	steps := twoSteps()
	send := wsend("s")
	send.Steps = steps
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), send}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "s")}
	f.r = f.runner(func(int) {
		for _, tk := range f.we.store.only().Tokens {
			if tk.NodeID == "s" && tk.ChildID != "" { // the run was started and its id saved
				die()
			}
		}
	})
	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, Rows: sheetRows(4)}); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if len(f.re.runs.runs) != 1 {
		t.Fatalf("one run for the one chunk, before and after the restart: %d", len(f.re.runs.runs))
	}
	if len(f.re.line.calls) != 3*len(steps) {
		t.Fatalf("each of the 3 KOCs got each step once: %d messages", len(f.re.line.calls))
	}
	if got := f.we.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestRestartWhileWaitingKeepsTheRemainingWait(t *testing.T) {
	f := newFlowEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 120)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "w")}
	f.r = f.runner(func(naps int) {
		if naps == 60 { // an hour into the two-hour wait
			die()
		}
	})
	wf := &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, By: "a@x.vn", Source: "TikToday"}
	if _, err := f.r.begin(context.Background(), wf, []domain.WorkflowToken{{NodeID: "w", Kocs: tokenKocs("k0"), ReadyAt: f.we.clock.Add(120 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if got := f.we.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if f.elapsed() < 120*time.Minute || f.elapsed() > 123*time.Minute {
		t.Fatalf("the wait ended two hours after it began, not %v (the restart does not start it again)", f.elapsed())
	}
}
