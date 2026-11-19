package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type startRecord struct {
	at       time.Time
	size     int
	nodeID   string
	switchOn bool
}

// fakeWfRuns stands for the campaign runner and its store. A run stays "running" until the test calls settle, which ends
// it the way the real runner would: it sends in order, stops at the daily limit, and turns the switch off at the end.
type fakeWfRuns struct {
	mu         sync.Mutex
	clock      func() time.Time
	sw         *memSwitch
	runs       []*domain.BulkRun
	starts     []startRecord
	maxRunning int

	dailyLimit      int               // KOCs a day (0 = no limit)
	sentOn          map[string]int    // KOCs sent on a Vietnam day
	script          map[string]string // the status of a KOC's target (sent when absent)
	forceStop       string            // the next run that settles stops with this reason
	busyFor         int               // the first N starts answer "a run is already running"
	ignoreWorkflows bool              // FindByWorkflow matches the block only (the id of the workflow is not known to a test)
}

func (f *fakeWfRuns) running() int {
	n := 0
	for _, r := range f.runs {
		if r.Status == domain.RunRunning {
			n++
		}
	}
	return n
}

func (f *fakeWfRuns) Start(_ context.Context, by, name string, steps []domain.CampaignStep, targets []domain.RunTarget, opts ...RunOptions) (*domain.BulkRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.sw.on {
		return nil, ErrBulkOff
	}
	if f.busyFor > 0 {
		f.busyFor--
		return nil, domain.Conflict("Đang có một đợt gửi chạy (lỗi của kho thật).")
	}
	if f.running() > 0 {
		return nil, domain.Conflict("Đang có một đợt gửi chạy (lỗi của kho thật).")
	}
	run := &domain.BulkRun{ID: domain.NewID(), CampaignName: name, Steps: steps, By: by, Status: domain.RunRunning}
	for _, t := range targets {
		run.Targets = append(run.Targets, domain.RunTarget{KocID: t.KocID, Status: domain.TargetQueued})
	}
	if len(opts) > 0 {
		run.WorkflowID, run.NodeID = opts[0].WorkflowID, opts[0].NodeID
	}
	f.runs = append(f.runs, run)
	f.starts = append(f.starts, startRecord{at: f.clock(), size: len(targets), nodeID: run.NodeID, switchOn: f.sw.on})
	if n := f.running(); n > f.maxRunning {
		f.maxRunning = n
	}
	cp := *run
	return &cp, nil
}

func (f *fakeWfRuns) FindByWorkflow(_ context.Context, workflowID, nodeID string) (*domain.BulkRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.runs) - 1; i >= 0; i-- {
		if r := f.runs[i]; r.NodeID == nodeID && (f.ignoreWorkflows || r.WorkflowID == workflowID) {
			cp := *r
			cp.Targets = append([]domain.RunTarget(nil), r.Targets...)
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeWfRuns) Running(context.Context) (*domain.BulkRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.runs {
		if r.Status == domain.RunRunning {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeWfRuns) Find(_ context.Context, id domain.ID) (*domain.BulkRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.runs {
		if r.ID == id {
			cp := *r
			cp.Targets = append([]domain.RunTarget(nil), r.Targets...)
			return &cp, nil
		}
	}
	return nil, domain.NotFound("không có")
}

// settle ends every running run.
func (f *fakeWfRuns) settle(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.runs {
		if r.Status != domain.RunRunning {
			continue
		}
		reason := ""
		if f.forceStop != "" {
			reason, f.forceStop = f.forceStop, ""
		} else {
			for i := range r.Targets {
				t := &r.Targets[i]
				status := domain.TargetSent
				if s, ok := f.script[t.KocID]; ok {
					status = s
				}
				day := vnDayKey(now)
				if status == domain.TargetSent && f.dailyLimit > 0 && f.sentOn[day] >= f.dailyLimit {
					reason = "daily_koc_limit_reached"
					break
				}
				t.Status = status
				at := now
				if status == domain.TargetSent || status == domain.TargetReplied {
					t.StartedAt = &at
				}
				if status == domain.TargetSent {
					f.sentOn[day]++
				}
			}
		}
		for i := range r.Targets {
			if r.Targets[i].Status == domain.TargetQueued {
				r.Targets[i].Status, r.Targets[i].Reason = domain.TargetNotSent, reason // what the repository does when a run ends
			}
		}
		if reason == "" {
			r.Status = domain.RunDone
		} else {
			r.Status, r.StopReason = domain.RunStopped, reason
		}
		f.sw.on = false // the real runner turns the switch off when a run ends
	}
}

func wsend(id string) domain.WorkflowNode {
	return domain.WorkflowNode{ID: id, Type: domain.NodeSend, CampaignID: "c1", CampaignName: "Mời",
		Steps: []domain.CampaignStep{{Kind: domain.StepText, Text: "a"}}}
}

func kocTokens(n int) []domain.TokenKoc {
	out := make([]domain.TokenKoc, n)
	for i := range out {
		out[i] = domain.TokenKoc{KocID: "k" + strconv.Itoa(i)}
	}
	return out
}

func sendGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), wsend("s")}, []domain.WorkflowEdge{wedge("i", domain.PortOut, "s")}
}

func (e *wfEnv) settleEveryTick() {
	e.onNap = func(int) { e.runs.settle(e.clock) }
}

func (e *wfEnv) startsAt() []time.Time {
	var out []time.Time
	for _, s := range e.runs.starts {
		out = append(out, s.at)
	}
	return out
}

func TestSendBlockSendsInChunksOfFifty(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	e.settleEveryTick()
	wf := e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(120), ReadyAt: e.clock})

	var sizes []int
	var order []string
	for i, s := range e.runs.starts {
		sizes = append(sizes, s.size)
		if want := "s#" + strconv.Itoa(i+1); s.nodeID != want {
			t.Fatalf("run %d is tagged %q, want %q", i+1, s.nodeID, want)
		}
		for _, tg := range e.runs.runs[i].Targets {
			order = append(order, tg.KocID)
		}
		if e.runs.runs[i].WorkflowID != wf.ID.Hex() {
			t.Fatalf("run %d is tagged with its workflow", i+1)
		}
	}
	if len(sizes) != 3 || sizes[0] != 50 || sizes[1] != 50 || sizes[2] != 20 {
		t.Fatalf("chunks of 50, 50 and 20: %v", sizes)
	}
	for i, id := range order {
		if id != "k"+strconv.Itoa(i) {
			t.Fatalf("the KOCs go out in order: position %d is %s", i, id)
		}
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("s"); s.Status != domain.NodeDone || s.Count != 120 {
		t.Fatalf("the send block shows done with 120 KOCs: %+v", s)
	}
}

func TestSendBlockDoesNotStartTwoRunsForTheSameChunk(t *testing.T) {
	existing := func(e *wfEnv, status string) *domain.BulkRun {
		r := &domain.BulkRun{ID: domain.NewID(), NodeID: "s#1", Status: status}
		for _, k := range kocTokens(3) {
			r.Targets = append(r.Targets, domain.RunTarget{KocID: k.KocID, Status: domain.TargetQueued})
		}
		e.runs.runs = append(e.runs.runs, r)
		e.runs.ignoreWorkflows = true
		return r
	}
	t.Run("the block is found by its tags after a restart", func(t *testing.T) {
		e := newWfEnv()
		nodes, edges := sendGraph()
		existing(e, domain.RunRunning)
		e.sw.on = true
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(3), Chunk: 1, ReadyAt: e.clock})
		if len(e.runs.starts) != 0 {
			t.Fatalf("a run the block already started is followed, not started again: %d starts", len(e.runs.starts))
		}
		if got := e.store.only(); got.Status != domain.WorkflowDone {
			t.Fatalf("%s %q", got.Status, got.StopReason)
		}
	})
	t.Run("a run that already ended is not run again", func(t *testing.T) {
		e := newWfEnv()
		nodes, edges := sendGraph()
		r := existing(e, domain.RunDone)
		for i := range r.Targets {
			r.Targets[i].Status = domain.TargetSent
		}
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(3), Chunk: 1, ReadyAt: e.clock})
		if len(e.runs.starts) != 0 {
			t.Fatalf("the chunk was sent before the restart: nothing is started: %d starts", len(e.runs.starts))
		}
		if s, _ := e.stateOf("s"); s.Count != 3 {
			t.Fatalf("its KOCs are counted as sent: %+v", s)
		}
	})
	t.Run("the run is followed by its id", func(t *testing.T) {
		e := newWfEnv()
		nodes, edges := sendGraph()
		r := existing(e, domain.RunRunning)
		e.sw.on = true
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(3), Chunk: 1, ChildID: r.ID.Hex(), ReadyAt: e.clock})
		if len(e.runs.starts) != 0 {
			t.Fatalf("%d starts", len(e.runs.starts))
		}
		if got := e.store.only(); got.Status != domain.WorkflowDone {
			t.Fatalf("%s %q", got.Status, got.StopReason)
		}
	})
	t.Run("a chunk that was decided but never started is started once", func(t *testing.T) {
		e := newWfEnv()
		nodes, edges := sendGraph()
		e.settleEveryTick()
		e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(3), Chunk: 1, ReadyAt: e.clock})
		if len(e.runs.starts) != 1 || e.runs.starts[0].nodeID != "s#1" {
			t.Fatalf("one start, tagged with the chunk that was decided: %+v", e.runs.starts)
		}
	})
}

func TestSendBlockWaitsForTheNextWindowOutsideOfIt(t *testing.T) {
	e := newWfEnv()
	e.clock = time.Date(2026, 11, 20, 22, 0, 0, 0, vnZone)
	e.t0 = e.clock
	nodes, edges := sendGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(2), ReadyAt: e.clock})
	at := e.startsAt()
	if len(at) != 1 {
		t.Fatalf("one run: %v", at)
	}
	open := time.Date(2026, 11, 21, 8, 30, 0, 0, vnZone)
	if at[0].Before(open) || at[0].After(open.Add(3*time.Minute)) {
		t.Fatalf("the run starts when the window opens (%v), not at %v", open, at[0])
	}
}

func TestSendBlockWaitsForTheWindowBeforeDawn(t *testing.T) {
	e := newWfEnv()
	e.clock = time.Date(2026, 11, 20, 6, 0, 0, 0, vnZone)
	e.t0 = e.clock
	nodes, edges := sendGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(2), ReadyAt: e.clock})
	open := time.Date(2026, 11, 20, 8, 30, 0, 0, vnZone)
	if at := e.startsAt(); len(at) != 1 || at[0].Before(open) || at[0].After(open.Add(3*time.Minute)) {
		t.Fatalf("before 08:30 it waits for the same day's window: %v", at)
	}
}

func TestSendBlockContinuesTomorrowAfterTheDailyLimit(t *testing.T) {
	e := newWfEnv()
	e.runs.dailyLimit = 3
	nodes, edges := sendGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(5), ReadyAt: e.clock})
	if len(e.runs.starts) != 2 || e.runs.starts[0].size != 5 || e.runs.starts[1].size != 2 {
		t.Fatalf("5 KOCs, then only the 2 that were not sent: %+v", e.runs.starts)
	}
	second := e.runs.runs[1].Targets
	if second[0].KocID != "k3" || second[1].KocID != "k4" {
		t.Fatalf("the KOCs that were already sent are not sent again: %+v", second)
	}
	if next := time.Date(2026, 11, 21, 8, 30, 0, 0, vnZone); e.runs.starts[1].at.Before(next) {
		t.Fatalf("the rest goes out when tomorrow's window opens, not at %v", e.runs.starts[1].at)
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if s, _ := e.stateOf("s"); s.Count != 5 {
		t.Fatalf("all 5 are counted: %+v", s)
	}
}

func TestSendBlockStopsTheWorkflowOnOtherStopReasons(t *testing.T) {
	for _, reason := range []string{"switch_off", "zalo_errors", "zalo_not_ready", "store_unavailable", "internal_error"} {
		t.Run(reason, func(t *testing.T) {
			e := newWfEnv()
			nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wsend("s"), wwait("w", 5)}
			edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "s"), wedge("s", domain.PortOut, "w")}
			e.runs.forceStop = reason
			e.settleEveryTick()
			e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(4), ReadyAt: e.clock})
			got := e.store.only()
			if got.Status != domain.WorkflowStopped || got.StopReason != "send_stopped:"+reason {
				t.Fatalf("%s %q", got.Status, got.StopReason)
			}
			if _, entered := e.stateOf("w"); entered {
				t.Fatal("the next block never started")
			}
			if len(e.runs.starts) != 1 {
				t.Fatalf("no second run after a stop: %d", len(e.runs.starts))
			}
		})
	}
}

func TestSendBlockKeepsOnlySentAndRepliedKocsForTheNextBlock(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wsend("s"), wwait("w", 5)}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "s"), wedge("s", domain.PortOut, "w")}
	e.runs.script = map[string]string{"k1": domain.TargetReplied, "k2": domain.TargetSkipped, "k3": domain.TargetFailed, "k4": domain.TargetPartial, "k5": domain.TargetUnknown}
	var carried []domain.TokenKoc
	e.onNap = func(int) {
		e.runs.settle(e.clock)
		for _, tk := range e.store.only().Tokens {
			if tk.NodeID == "w" && carried == nil {
				carried = tk.Kocs
			}
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(6), ReadyAt: e.clock})
	if len(carried) != 2 || carried[0].KocID != "k0" || carried[1].KocID != "k1" {
		t.Fatalf("only the KOCs that were sent to go on: %+v", carried)
	}
	if carried[0].SentAt == nil || carried[1].SentAt == nil {
		t.Fatalf("each carries the moment its steps went out: %+v", carried)
	}
	if s, _ := e.stateOf("s"); !strings.Contains(s.Reason, "4 bỏ qua") || s.Count != 2 {
		t.Fatalf("the block says how many dropped out: %+v", s)
	}
}

func TestSendBlockSwitchesTheBulkSwitchOnBeforeEveryRun(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(60), ReadyAt: e.clock})
	if len(e.runs.starts) != 2 {
		t.Fatalf("two runs for 60 KOCs: %d", len(e.runs.starts))
	}
	for i, s := range e.runs.starts {
		if !s.switchOn {
			t.Fatalf("run %d started with the switch off", i+1)
		}
	}
}

func TestTwoBranchesNeverStartTwoRunsAtOnce(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wsend("sA"), wsend("sB")}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortReplied, "sA"), wedge("q", domain.PortUnanswered, "sB")}
	e.settleEveryTick()
	e.begin(nodes, edges,
		domain.WorkflowToken{NodeID: "sA", Kocs: kocTokens(3), ReadyAt: e.clock},
		domain.WorkflowToken{NodeID: "sB", Kocs: []domain.TokenKoc{{KocID: "b0"}, {KocID: "b1"}}, ReadyAt: e.clock})
	if e.runs.maxRunning != 1 {
		t.Fatalf("never two runs at once: %d", e.runs.maxRunning)
	}
	if len(e.runs.starts) != 2 {
		t.Fatalf("both branches got their run: %+v", e.runs.starts)
	}
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	for _, id := range []string{"sA", "sB"} {
		if s, _ := e.stateOf(id); s.Status != domain.NodeDone {
			t.Fatalf("block %s: %+v", id, s)
		}
	}
}

func TestSendBlockGivesUpAfterAnHourOfWaitingForAnotherRun(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	e.runs.busyFor = 100000
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(2), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "send_busy" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if e.elapsed() < 59*time.Minute || e.elapsed() > 65*time.Minute {
		t.Fatalf("it gave up after about an hour, not %v", e.elapsed())
	}
}

func TestStoppingTheWorkflowSwitchesTheBulkSwitchOff(t *testing.T) {
	e := newWfEnv()
	nodes, edges := sendGraph()
	switchWasOn := false
	e.onNap = func(n int) { // the run never ends on its own here
		if n == 3 {
			switchWasOn = e.sw.on
			e.store.askToStop()
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(5), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "stopped_by_user" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if !switchWasOn || e.sw.on {
		t.Fatalf("the switch was on while the run ran (%v) and is off after the stop (%v)", switchWasOn, e.sw.on)
	}
}

func TestTwoHundredKocsFinishWithinFourteenDays(t *testing.T) {
	e := newWfEnv()
	e.runs.dailyLimit = 30
	nodes, edges := sendGraph()
	e.settleEveryTick()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "s", Kocs: kocTokens(200), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q after %v", got.Status, got.StopReason, e.elapsed())
	}
	if e.elapsed() > 8*24*time.Hour {
		t.Fatalf("200 KOCs at 30 a day take 7 days, not %v", e.elapsed())
	}
	seen := map[string]int{}
	for _, r := range e.runs.runs {
		if len(r.Targets) > MaxRunKocs {
			t.Fatalf("a run of %d KOCs", len(r.Targets))
		}
		for _, tg := range r.Targets {
			if tg.Status == domain.TargetSent {
				seen[tg.KocID]++
			}
		}
	}
	if len(seen) != 200 {
		t.Fatalf("every KOC was sent to: %d", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("KOC %s was sent to %d times", id, n)
		}
	}
	if s, _ := e.stateOf("s"); s.Count != 200 {
		t.Fatalf("the block counts 200: %+v", s)
	}
}
