package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// workflowTick is how often the runner looks at its branches: whether one of them is due, and whether somebody asked it
// to stop.
const workflowTick = 5 * time.Second

// ErrWorkflowInProgress is what Start answers while another workflow is running.
var ErrWorkflowInProgress = domain.Conflict("Đang có một quy trình chạy. Hãy chờ nó xong hoặc dừng nó.")

// WorkflowStore is where workflows are kept (*repository.WorkflowRepository).
type WorkflowStore interface {
	Create(ctx context.Context, wf *domain.Workflow) error
	Running(ctx context.Context) (*domain.Workflow, error)
	SaveTokens(ctx context.Context, id domain.ID, tokens []domain.WorkflowToken) error
	SetState(ctx context.Context, id domain.ID, st domain.NodeState) error
	StopRequested(ctx context.Context, id domain.ID) (bool, error)
	Finish(ctx context.Context, id domain.ID, status, reason string, at time.Time) error
}

// WorkflowImports starts and follows the import jobs of the import blocks (*ImportRunner and *repository.ImportJobRepository).
type WorkflowImports interface {
	Start(ctx context.Context, by, source string, rows []domain.ImportRow, opts ...ImportOptions) (*domain.ImportJob, error)
	FindByWorkflow(ctx context.Context, workflowID, nodeID string) (*domain.ImportJob, error)
	Find(ctx context.Context, id string) (*domain.ImportJob, error)
	RequestStop(ctx context.Context, id string) (bool, error)
}

// WorkflowRuns starts and follows the bulk runs of the send blocks (*CampaignRunner and *repository.BulkRunRepository).
type WorkflowRuns interface {
	Start(ctx context.Context, by, campaignName string, steps []domain.CampaignStep, targets []domain.RunTarget, opts ...RunOptions) (*domain.BulkRun, error)
	FindByWorkflow(ctx context.Context, workflowID, nodeID string) (*domain.BulkRun, error)
	Find(ctx context.Context, id domain.ID) (*domain.BulkRun, error)
	Running(ctx context.Context) (*domain.BulkRun, error)
}

// ReplyChecker says whether a KOC answered after a given moment (*OutreachService). known is false when nobody can tell.
type ReplyChecker interface {
	RepliedSince(ctx context.Context, kocID string, since time.Time) (replied bool, known bool, err error)
}

// WorkflowRunner runs the workflows in the background. A workflow is a graph; every branch that is running is a token that
// sits at one block, carries a list of KOCs and may go on from a given time. Waiting branches do not hold each other: each
// token is looked at on its own. The runner writes every move into the workflow, and the screen only reads it, so closing
// the page or restarting the server changes nothing: Resume goes on from the tokens that were saved.
type WorkflowRunner struct {
	store   WorkflowStore
	imports WorkflowImports
	runs    WorkflowRuns
	sw      BulkSwitch
	replies ReplyChecker
	picker  KocPicker

	tick time.Duration
	// tests replace these
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time

	wg sync.WaitGroup
}

// NewWorkflowRunner builds the runner.
func NewWorkflowRunner(store WorkflowStore, imports WorkflowImports, runs WorkflowRuns, sw BulkSwitch, replies ReplyChecker) *WorkflowRunner {
	return &WorkflowRunner{store: store, imports: imports, runs: runs, sw: sw, replies: replies,
		tick: workflowTick, sleep: realSleep, now: time.Now}
}

// Wait blocks until every background workflow that was started has ended (for tests and for a clean shutdown).
func (r *WorkflowRunner) Wait() { r.wg.Wait() }

// Start checks a workflow, stores it and begins it in the background: the first token sits at the import block. It refuses
// a graph that cannot run, a sheet with no row or too many, and a sheet where every row was refused, and it answers
// ErrWorkflowInProgress while another workflow is running. The caller gets a copy: the worker owns the workflow.
func (r *WorkflowRunner) Start(ctx context.Context, by string, wf *domain.Workflow) (*domain.Workflow, error) {
	if err := domain.ValidateWorkflowGraph(wf.Nodes, wf.Edges); err != nil {
		return nil, err
	}
	var start domain.WorkflowNode
	for _, n := range wf.Nodes {
		if domain.IsStartNode(n.Type) {
			start = n
		}
	}
	first := domain.WorkflowToken{NodeID: start.ID, ReadyAt: r.now()}
	if start.Type == domain.NodePick {
		// no sheet: the KOCs come from the directory, and the list is taken NOW, once, so that it does not change with a restart
		if r.picker == nil {
			return nil, domain.Invalid("Chưa cấu hình được việc lấy KOC có sẵn.")
		}
		kocs, err := r.picker.Pick(ctx, start.Stages, start.Folder, start.Limit)
		if err != nil {
			return nil, err
		}
		if len(kocs) == 0 {
			return nil, domain.Invalid("Không có KOC nào khớp để lấy.")
		}
		wf.Rows, first.Kocs = nil, kocs
	} else {
		if len(wf.Rows) == 0 || len(wf.Rows) > domain.MaxWorkflowRows {
			return nil, domain.Invalid("Mỗi lần nhập từ 1 đến 200 dòng.")
		}
		queued := 0
		for i := range wf.Rows {
			if wf.Rows[i].Status == domain.RowInvalid { // refused before: kept with its reason, never imported
				continue
			}
			wf.Rows[i].Status, wf.Rows[i].Reason = domain.RowQueued, ""
			queued++
		}
		if queued == 0 {
			return nil, domain.Invalid("Không có dòng nào hợp lệ để nhập.")
		}
	}
	wf.By = by
	if wf.Source == "" {
		wf.Source = "TikToday"
	}
	return r.begin(ctx, wf, []domain.WorkflowToken{first})
}

// WithPicker gives the runner the way to take KOCs from the directory for a pick block.
func (r *WorkflowRunner) WithPicker(p KocPicker) *WorkflowRunner {
	r.picker = p
	return r
}

// begin stores the workflow with its first tokens and starts working on it.
func (r *WorkflowRunner) begin(ctx context.Context, wf *domain.Workflow, tokens []domain.WorkflowToken) (*domain.Workflow, error) {
	wf.Tokens = tokens
	if err := r.store.Create(ctx, wf); err != nil {
		return nil, err
	}
	for _, t := range tokens {
		r.markEntered(ctx, wf, t)
	}
	snapshot := cloneWorkflowFor(wf)
	r.spawn(wf)
	return snapshot, nil
}

func cloneWorkflowFor(wf *domain.Workflow) *domain.Workflow {
	cp := *wf
	cp.Nodes = append([]domain.WorkflowNode(nil), wf.Nodes...)
	cp.Edges = append([]domain.WorkflowEdge(nil), wf.Edges...)
	cp.Rows = append([]domain.ImportRow(nil), wf.Rows...)
	cp.Tokens = append([]domain.WorkflowToken(nil), wf.Tokens...)
	cp.States = append([]domain.NodeState(nil), wf.States...)
	return &cp
}

// Resume goes on with the workflow that was running when the server stopped.
func (r *WorkflowRunner) Resume(ctx context.Context) {
	wf, err := r.store.Running(ctx)
	if err != nil {
		log.Printf("workflow: cannot look for a workflow to resume: %v", err)
		return
	}
	if wf != nil {
		r.spawn(wf)
	}
}

func (r *WorkflowRunner) spawn(wf *domain.Workflow) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.work(wf)
	}()
}

func nodeOf(wf *domain.Workflow, id string) (domain.WorkflowNode, bool) {
	for _, n := range wf.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return domain.WorkflowNode{}, false
}

// newToken makes the token that arrives at a block: it may go on at once, except at a wait, which holds it for its time.
func (r *WorkflowRunner) newToken(wf *domain.Workflow, nodeID string, kocs []domain.TokenKoc) domain.WorkflowToken {
	t := domain.WorkflowToken{NodeID: nodeID, Kocs: kocs, ReadyAt: r.now()}
	if n, ok := nodeOf(wf, nodeID); ok {
		switch n.Type {
		case domain.NodeWait:
			t.ReadyAt = t.ReadyAt.Add(time.Duration(n.WaitMinutes) * time.Minute)
		case domain.NodeWaitUntil:
			t.ReadyAt = nextClockTime(t.ReadyAt, n.AtMinute)
		}
	}
	return t
}

// markEntered shows on the screen that a token arrived at its block.
func (r *WorkflowRunner) markEntered(ctx context.Context, wf *domain.Workflow, t domain.WorkflowToken) {
	st := domain.NodeState{NodeID: t.NodeID, Status: domain.NodeRunning, Count: len(t.Kocs)}
	if n, ok := nodeOf(wf, t.NodeID); ok && (n.Type == domain.NodeWait || n.Type == domain.NodeWaitUntil) {
		until := t.ReadyAt
		st.Status, st.WaitUntil = domain.NodeWaiting, &until
	}
	r.setState(ctx, wf, st)
}

func (r *WorkflowRunner) setState(ctx context.Context, wf *domain.Workflow, st domain.NodeState) {
	if err := r.store.SetState(ctx, wf.ID, st); err != nil {
		log.Printf("workflow %s: cannot record the state of block %s: %v", wf.ID, st.NodeID, err)
	}
}

// leave sends the KOCs on along the wire of a port. It returns the token that arrives at the next block, or none when the
// port has no wire or there is nobody to send: the branch ends there.
func (r *WorkflowRunner) leave(ctx context.Context, wf *domain.Workflow, from, port string, kocs []domain.TokenKoc) []domain.WorkflowToken {
	to, ok := domain.NextNode(wf.Edges, from, port)
	if !ok || len(kocs) == 0 {
		return nil
	}
	t := r.newToken(wf, to, kocs)
	r.markEntered(ctx, wf, t)
	return []domain.WorkflowToken{t}
}

// advanceResult is what happens to a token that is due: the tokens that replace it (none = the branch ends, the same token
// with a later time = it keeps waiting), or a reason to stop the whole workflow.
type advanceResult struct {
	tokens []domain.WorkflowToken
	stop   string
}

func (r *WorkflowRunner) advance(ctx context.Context, wf *domain.Workflow, t domain.WorkflowToken) advanceResult {
	n, ok := nodeOf(wf, t.NodeID)
	if !ok {
		return advanceResult{stop: "internal_error"}
	}
	switch n.Type {
	case domain.NodeImport:
		return r.advanceImport(ctx, wf, n, t)
	case domain.NodePick:
		return r.advancePick(ctx, wf, n, t)
	case domain.NodeSend:
		return r.advanceSend(ctx, wf, n, t)
	case domain.NodeIfReplied:
		return r.advanceIfReplied(ctx, wf, n, t)
	case domain.NodeWait, domain.NodeWaitUntil:
		r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Kocs)})
		return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Kocs)}
	}
	return advanceResult{stop: "unsupported_block:" + n.Type}
}

// stopTokens shows the blocks the branches were at as stopped.
func (r *WorkflowRunner) stopTokens(ctx context.Context, wf *domain.Workflow) {
	for _, t := range wf.Tokens {
		r.setState(ctx, wf, domain.NodeState{NodeID: t.NodeID, Status: domain.NodeStopped, Count: len(t.Kocs)})
	}
}

// work goes through the branches until none is left.
func (r *WorkflowRunner) work(wf *domain.Workflow) {
	ctx := context.Background()
	finished := false
	finish := func(status, reason string) {
		finished = true
		if status == domain.WorkflowStopped {
			r.stopChildren(ctx, wf)
			r.stopTokens(ctx, wf)
		}
		// a workflow that is never marked as ended would block every later one, so the end is tried again before giving up
		var err error
		for try := 1; try <= 5; try++ {
			if err = r.store.Finish(ctx, wf.ID, status, reason, r.now()); err == nil {
				return
			}
			_ = r.sleep(ctx, time.Duration(try)*time.Second)
		}
		log.Printf("workflow %s: cannot record the end: %v", wf.ID, err)
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("workflow %s: panic: %v", wf.ID, p)
			if !finished {
				finish(domain.WorkflowStopped, "internal_error")
			}
		}
	}()

	for {
		if stop, err := r.store.StopRequested(ctx, wf.ID); err == nil && stop {
			finish(domain.WorkflowStopped, "stopped_by_user")
			return
		}
		if r.now().Sub(wf.CreatedAt) > domain.MaxWorkflowAge {
			finish(domain.WorkflowStopped, "expired")
			return
		}
		for i := 0; i < len(wf.Tokens); {
			t := wf.Tokens[i]
			if t.ReadyAt.After(r.now()) {
				i++
				continue
			}
			res := r.advance(ctx, wf, t)
			if res.stop != "" {
				finish(domain.WorkflowStopped, res.stop)
				return
			}
			tail := append([]domain.WorkflowToken(nil), wf.Tokens[i+1:]...)
			wf.Tokens = append(append(wf.Tokens[:i:i], res.tokens...), tail...)
			if err := r.store.SaveTokens(ctx, wf.ID, wf.Tokens); err != nil {
				log.Printf("workflow %s: cannot save the branches: %v", wf.ID, err)
				finish(domain.WorkflowStopped, "internal_error")
				return
			}
			i += len(res.tokens) // a token that just moved is looked at again at the next tick
		}
		if len(wf.Tokens) == 0 {
			finish(domain.WorkflowDone, "")
			return
		}
		if err := r.sleep(ctx, r.tick); err != nil {
			finish(domain.WorkflowStopped, "internal_error")
			return
		}
	}
}
