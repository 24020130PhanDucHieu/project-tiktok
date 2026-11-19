package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// chunkTag is what the run of one chunk of a send block is tagged with: the block and the number of the chunk. The number
// is written into the token before the run is started, so after a restart the runner finds the run of exactly that chunk
// (or knows that none was started) and never starts a chunk twice.
func chunkTag(nodeID string, chunk int) string { return nodeID + "#" + strconv.Itoa(chunk) }

// nextSendWindowStart is the next moment the send window (08:30 Vietnam time) opens: today's when it has not opened yet,
// otherwise tomorrow's.
func nextSendWindowStart(now time.Time) time.Time {
	vn := now.In(vnZone)
	open := time.Date(vn.Year(), vn.Month(), vn.Day(), 8, 30, 0, 0, vnZone)
	if vn.Before(open) {
		return open
	}
	return open.AddDate(0, 0, 1)
}

// sendSteps is what a send block sends: the steps of its campaign, or, for a send_text block, its one text.
func sendSteps(n domain.WorkflowNode) (name string, steps []domain.CampaignStep) {
	if n.Type == domain.NodeSendText {
		return "Tin lẻ", []domain.CampaignStep{{Kind: domain.StepText, Text: n.Text}}
	}
	return n.CampaignName, n.Steps
}

// advanceSend sends the campaign of the block to the KOCs of the token, chunk by chunk (a run holds at most MaxRunKocs), one
// run at a time. A run that stops because of a daily limit or the end of the window leaves its KOCs for the next window.
func (r *WorkflowRunner) advanceSend(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	if t.ChildID != "" {
		return r.followRun(ctx, wf, n, t)
	}
	if len(t.Kocs) == 0 {
		return r.finishSend(ctx, wf, n, t)
	}
	if t.Chunk == 0 { // decide the chunk first: the token is saved before anything is started
		t.Chunk = 1
		return advanceResult{tokens: []domain.WorkflowToken{t}}
	}

	tag := chunkTag(n.ID, t.Chunk)
	run, err := r.runs.FindByWorkflow(ctx, wf.ID.Hex(), tag)
	if err != nil {
		log.Printf("workflow %s: cannot look for the run of block %s: %v", wf.ID, tag, err)
		return r.hold(t, r.tick)
	}
	if run != nil { // started before a restart: follow it
		t.ChildID, t.Waits = run.ID.Hex(), 0
		return r.hold(t, r.tick)
	}

	now := r.now()
	if !IsWithinAllowedSendWindow(now) {
		return r.hold(t, nextSendWindowStart(now).Sub(now))
	}
	// Another run is running: wait, and do not touch the switch. It is the stop button of that run, and turning it on again
	// here would undo a stop.
	if running, err := r.runs.Running(ctx); err != nil || running != nil {
		return r.busy(t)
	}
	chunk := t.Kocs
	if len(chunk) > MaxRunKocs {
		chunk = chunk[:MaxRunKocs]
	}
	// A KOC that answered since its steps last went out (while this block waited for the window, the daily limit or another
	// run) must not get this campaign: the check of the if-replied block can be hours or days old.
	if kept, dropped, retry := r.dropAnswered(ctx, wf, t, chunk); retry {
		t.Reads++
		return r.hold(t, r.tick)
	} else if dropped > 0 {
		t.Skipped += dropped
		t.Reads = 0
		t.Kocs = append(kept, t.Kocs[len(chunk):]...)
		if len(t.Kocs) == 0 {
			return r.finishSend(ctx, wf, n, t)
		}
		return advanceResult{tokens: []domain.WorkflowToken{t}} // the next look starts the shorter chunk
	}
	if err := r.sw.SetEnabled(ctx, true, "workflow"); err != nil {
		log.Printf("workflow %s: cannot turn the switch on: %v", wf.ID, err)
		return r.hold(t, r.tick)
	}
	targets := make([]domain.RunTarget, len(chunk))
	for i, k := range chunk {
		targets[i] = domain.RunTarget{KocID: k.KocID}
	}
	name, steps := sendSteps(n)
	run, err = r.runs.Start(ctx, wf.By, name, steps, targets, RunOptions{WorkflowID: wf.ID.Hex(), NodeID: tag})
	switch {
	case errors.Is(err, ErrBulkOff):
		return r.busy(t) // somebody turned the switch off again: it is turned on again at the next try
	case isBusy(err):
		return r.busy(t)
	case err != nil:
		log.Printf("workflow %s: cannot start the run of block %s: %v", wf.ID, tag, err)
		return advanceResult{stop: "send_failed"}
	}
	t.ChildID, t.Waits, t.Reads = run.ID.Hex(), 0, 0
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeRunning, Count: len(t.Done)})
	return r.hold(t, r.tick)
}

// dropAnswered keeps the KOCs of the chunk that did not answer since their steps last went out. A KOC that was never sent to
// by this workflow (it comes from the import) has nothing to check. One that cannot be checked is not messaged: a store that
// failed is read again a few times first (retry), then the KOC is given up.
func (r *WorkflowRunner) dropAnswered(ctx context.Context, wf *domain.Workflow, t domain.WorkflowToken, chunk []domain.TokenKoc) (kept []domain.TokenKoc, dropped int, retry bool) {
	if r.replies == nil {
		return chunk, 0, false
	}
	failed := 0
	for _, k := range chunk {
		if k.SentAt == nil {
			kept = append(kept, k)
			continue
		}
		did, known, err := r.replies.RepliedSince(ctx, k.KocID, *k.SentAt)
		switch {
		case err != nil:
			log.Printf("workflow %s: cannot tell whether KOC %s answered: %v", wf.ID, k.KocID, err)
			failed++
			dropped++
		case !known || did:
			dropped++
		default:
			kept = append(kept, k)
		}
	}
	if failed > 0 && t.Reads < maxReplyReadRetries {
		return nil, 0, true
	}
	return kept, dropped, false
}

// ownsRunningRun says whether the token of a send block has a run of its own that is still running: the one it follows, or,
// when the server died before that was written down, the one started for its chunk. When that cannot be read, the answer is
// yes: stopping must never leave a run going.
func (r *WorkflowRunner) ownsRunningRun(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) bool {
	var run *domain.BulkRun
	var err error
	switch {
	case t.ChildID != "":
		var id domain.ID
		if id, err = domain.ParseID(t.ChildID); err == nil {
			run, err = r.runs.Find(ctx, id)
		}
	case t.Chunk > 0:
		run, err = r.runs.FindByWorkflow(ctx, wf.ID.Hex(), chunkTag(n.ID, t.Chunk))
	default:
		return false
	}
	if err != nil {
		log.Printf("workflow %s: cannot tell whether the run of block %s still runs: %v", wf.ID, n.ID, err)
		return true
	}
	return run != nil && run.Status == domain.RunRunning
}

// busy keeps the token for a minute because another run is running, and gives up after an hour.
func (r *WorkflowRunner) busy(t domain.WorkflowToken) advanceResult {
	t.Waits++
	if t.Waits > maxBusyWaits {
		return advanceResult{stop: "send_busy"}
	}
	return r.hold(t, time.Minute)
}

// followRun watches the run of the current chunk and, when it ended, takes the KOCs it sent and decides what happens next.
func (r *WorkflowRunner) followRun(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	id, err := domain.ParseID(t.ChildID)
	if err != nil {
		return advanceResult{stop: "internal_error"}
	}
	run, err := r.runs.Find(ctx, id)
	if err != nil {
		log.Printf("workflow %s: cannot read the run %s: %v", wf.ID, t.ChildID, err)
		return r.hold(t, r.tick)
	}
	if run.Status == domain.RunRunning {
		sent := 0
		for _, tg := range run.Targets {
			if tg.Status == domain.TargetSent || tg.Status == domain.TargetReplied {
				sent++
			}
		}
		r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeRunning, Count: len(t.Done) + sent})
		return r.hold(t, r.tick)
	}

	// What the token knew about each KOC (above all, when it was last sent to) is kept for the KOCs that go out in a later run.
	known := map[string]domain.TokenKoc{}
	for _, k := range t.Kocs {
		if _, dup := known[k.KocID]; !dup {
			known[k.KocID] = k
		}
	}
	var again []domain.TokenKoc // not reached by the run: they go out in the next one
	inRun := map[string]bool{}
	for _, tg := range run.Targets {
		inRun[tg.KocID] = true
		switch tg.Status {
		case domain.TargetSent, domain.TargetReplied:
			t.Done = append(t.Done, domain.TokenKoc{KocID: tg.KocID, SentAt: tg.StartedAt})
		case domain.TargetQueued, domain.TargetNotSent:
			if k, ok := known[tg.KocID]; ok {
				again = append(again, k)
			} else {
				again = append(again, domain.TokenKoc{KocID: tg.KocID})
			}
		default: // skipped, failed, partial, unknown: nothing more is done for them, so nothing could be sent twice
			t.Skipped++
		}
	}
	// The KOCs that are left are found by id, not by position: the run may hold fewer KOCs than the chunk the token still lists
	// (some were dropped before it started, and the server may have died before the token was saved).
	var rest []domain.TokenKoc
	listed := map[string]bool{}
	for _, k := range t.Kocs {
		if !inRun[k.KocID] && !listed[k.KocID] {
			listed[k.KocID] = true
			rest = append(rest, k)
		}
	}
	t.Kocs = append(again, rest...)
	t.ChildID = ""

	resumeAt := r.now()
	if run.Status == domain.RunStopped {
		switch run.StopReason {
		case "daily_koc_limit_reached", "daily_message_cap_reached", "outside_0830_2030_window":
			resumeAt = nextSendWindowStart(r.now())
		default:
			return advanceResult{stop: "send_stopped:" + run.StopReason}
		}
	}
	if len(t.Kocs) == 0 {
		return r.finishSend(ctx, wf, n, t)
	}
	t.Chunk++
	t.ReadyAt, t.Waits = resumeAt, 0
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeWaiting, Count: len(t.Done), WaitUntil: &resumeAt})
	return advanceResult{tokens: []domain.WorkflowToken{t}}
}

// finishSend passes on the KOCs that were sent to.
func (r *WorkflowRunner) finishSend(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	st := domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Done)}
	if t.Skipped > 0 {
		st.Reason = fmt.Sprintf("%d bỏ qua", t.Skipped)
	}
	r.setState(ctx, wf, st)
	return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Done)}
}
