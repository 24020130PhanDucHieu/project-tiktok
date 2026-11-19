package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeWfImports stands for the import runner and its store: the test decides when a job ends.
type fakeWfImports struct {
	mu          sync.Mutex
	jobs        []*domain.ImportJob
	starts      int
	busyFor     int               // the first N starts answer "an import is already running"
	preexisting *domain.ImportJob // a job a previous run of the server started for this block
	stopped     []string          // ids of the jobs the workflow asked to stop
}

func (f *fakeWfImports) Start(_ context.Context, by, source string, rows []domain.ImportRow, opts ...ImportOptions) (*domain.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busyFor > 0 {
		f.busyFor--
		return nil, ErrImportInProgress
	}
	job := &domain.ImportJob{ID: domain.NewID(), Source: source, By: by, Status: domain.ImportRunning, Rows: append([]domain.ImportRow(nil), rows...)}
	if len(opts) > 0 {
		job.WorkflowID, job.NodeID = opts[0].WorkflowID, opts[0].NodeID
	}
	f.jobs = append(f.jobs, job)
	f.starts++
	cp := *job
	return &cp, nil
}

func (f *fakeWfImports) FindByWorkflow(_ context.Context, workflowID, nodeID string) (*domain.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.preexisting != nil {
		cp := *f.preexisting
		return &cp, nil
	}
	for i := len(f.jobs) - 1; i >= 0; i-- {
		if j := f.jobs[i]; j.WorkflowID == workflowID && j.NodeID == nodeID {
			cp := *j
			cp.Rows = append([]domain.ImportRow(nil), j.Rows...)
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeWfImports) Find(_ context.Context, id string) (*domain.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, j := range append([]*domain.ImportJob{f.preexisting}, f.jobs...) {
		if j != nil && j.ID.Hex() == id {
			cp := *j
			cp.Rows = append([]domain.ImportRow(nil), j.Rows...)
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeWfImports) RequestStop(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
	return true, nil
}

// end finishes the only job: every row gets the status given for it (an added row also gets the id of a KOC).
func (f *fakeWfImports) end(status, reason string, rows ...domain.ImportRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var j *domain.ImportJob
	if f.preexisting != nil {
		j = f.preexisting
	} else {
		j = f.jobs[0]
	}
	for i := range rows {
		j.Rows[i] = rows[i]
	}
	j.Status, j.StopReason = status, reason
}

func addedRow(line int, kocID string) domain.ImportRow {
	return domain.ImportRow{Line: line, TiktokID: "k", Status: domain.RowAdded, KocID: kocID}
}
func duplicateRow(line int) domain.ImportRow {
	return domain.ImportRow{Line: line, TiktokID: "k", Status: domain.RowDuplicate}
}

func sheet(n int) []domain.ImportRow {
	out := make([]domain.ImportRow, n)
	for i := range out {
		out[i] = domain.ImportRow{Line: i + 2, TiktokID: "k", Status: domain.RowQueued}
	}
	return out
}

func importThenWait() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 5)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "w")}
}

// run starts a workflow from its first block, with a sheet, and waits for it to end.
func (e *wfEnv) run(nodes []domain.WorkflowNode, edges []domain.WorkflowEdge, rows []domain.ImportRow) *domain.Workflow {
	if _, err := e.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "w", Nodes: nodes, Edges: edges, Rows: rows}); err != nil {
		panic(err)
	}
	e.r.Wait()
	return e.store.only()
}

func TestImportBlockStartsOneJobAndPassesTheAddedKocs(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	var carried []domain.TokenKoc
	e.onNap = func(n int) {
		if n == 3 {
			e.imports.end(domain.ImportDone, "", addedRow(2, "A"), duplicateRow(3), addedRow(4, "B"))
		}
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == "w" && carried == nil {
				carried = tk.Kocs
			}
		}
	}
	got := e.run(nodes, edges, sheet(3))
	if e.imports.starts != 1 {
		t.Fatalf("one import job is started: %d", e.imports.starts)
	}
	job := e.imports.jobs[0]
	if job.WorkflowID != got.ID.Hex() || job.NodeID != "i" || job.By != "admin@t.t" {
		t.Fatalf("the job is tagged with its workflow and block: %+v", job)
	}
	if len(carried) != 2 || carried[0].KocID != "A" || carried[1].KocID != "B" {
		t.Fatalf("the KOCs of the added rows go on, in the order of the sheet: %+v", carried)
	}
	if got.Status != domain.WorkflowDone {
		t.Fatalf("the workflow ends done after the wait: %s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("i"); s.Status != domain.NodeDone || s.Count != 2 {
		t.Fatalf("the import block shows done with 2 KOCs: %+v", s)
	}
}

func TestImportBlockResumesAnExistingJobInsteadOfStartingAnother(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.imports.preexisting = &domain.ImportJob{ID: domain.NewID(), Status: domain.ImportRunning, Rows: sheet(2)}
	e.onNap = func(n int) {
		if n == 2 {
			e.imports.end(domain.ImportDone, "", addedRow(2, "A"), addedRow(3, "B"))
		}
	}
	got := e.begin(nodes, edges, domain.WorkflowToken{NodeID: "i", ReadyAt: e.clock})
	_ = got
	if e.imports.starts != 0 {
		t.Fatalf("a job the block already started is followed, not started again: %d starts", e.imports.starts)
	}
	if final := e.store.only(); final.Status != domain.WorkflowDone {
		t.Fatalf("it went on to the end: %s %q", final.Status, final.StopReason)
	}
}

func TestAStoppedImportStopsTheWorkflowWithItsReason(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.onNap = func(n int) {
		if n == 2 {
			e.imports.end(domain.ImportStopped, "rate_limited", addedRow(2, "A"), domain.ImportRow{Line: 3, TiktokID: "k", Status: domain.RowQueued})
		}
	}
	got := e.run(nodes, edges, sheet(2))
	if got.Status != domain.WorkflowStopped || got.StopReason != "import_stopped:rate_limited" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if _, entered := e.stateOf("w"); entered {
		t.Fatal("the next block never started")
	}
}

func TestImportBlockWaitsWhileAnotherImportRunsAndThenStarts(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.imports.busyFor = 3
	e.onNap = func(n int) {
		e.imports.mu.Lock()
		running := len(e.imports.jobs) > 0 && e.imports.jobs[0].Status == domain.ImportRunning
		e.imports.mu.Unlock()
		if running {
			e.imports.end(domain.ImportDone, "", addedRow(2, "A"))
		}
	}
	got := e.run(nodes, edges, sheet(1))
	if got.Status != domain.WorkflowDone || e.imports.starts != 1 {
		t.Fatalf("it started once the other import was gone: %s %q starts=%d", got.Status, got.StopReason, e.imports.starts)
	}
	if e.elapsed() < 3*time.Minute {
		t.Fatalf("it waited a minute for each refusal: %v", e.elapsed())
	}
}

func TestImportBlockGivesUpAfterAnHourOfWaiting(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.imports.busyFor = 100000
	got := e.run(nodes, edges, sheet(1))
	if got.Status != domain.WorkflowStopped || got.StopReason != "import_busy" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if e.elapsed() < 59*time.Minute || e.elapsed() > 65*time.Minute {
		t.Fatalf("it gave up after about an hour, not %v", e.elapsed())
	}
}

func TestStoppingTheWorkflowAsksTheRunningImportToStop(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.onNap = func(n int) {
		if n == 3 {
			e.store.askToStop()
		}
	}
	got := e.run(nodes, edges, sheet(2))
	if got.Status != domain.WorkflowStopped || got.StopReason != "stopped_by_user" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if len(e.imports.stopped) != 1 || e.imports.stopped[0] != e.imports.jobs[0].ID.Hex() {
		t.Fatalf("the running job was asked to stop: %v", e.imports.stopped)
	}
}

func TestAnImportWithNoAddedKocEndsTheBranch(t *testing.T) {
	e := newWfEnv()
	nodes, edges := importThenWait()
	e.onNap = func(n int) {
		if n == 2 {
			e.imports.end(domain.ImportDone, "", duplicateRow(2), duplicateRow(3))
		}
	}
	got := e.run(nodes, edges, sheet(2))
	if got.Status != domain.WorkflowDone {
		t.Fatalf("nothing to send: the workflow is done, not stuck: %s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("i"); s.Reason != "empty_list" || s.Count != 0 {
		t.Fatalf("the import block says why the branch ended: %+v", s)
	}
	if _, entered := e.stateOf("w"); entered {
		t.Fatal("the wait was never entered")
	}
}
