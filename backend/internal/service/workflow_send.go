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
	if err := r.sw.SetEnabled(ctx, true, "workflow"); err != nil {
		log.Printf("workflow %s: cannot turn the switch on: %v", wf.ID, err)
		return r.hold(t, r.tick)
	}
	chunk := t.Kocs
	if len(chunk) > MaxRunKocs {
		chunk = chunk[:MaxRunKocs]
	}
	targets := make([]domain.RunTarget, len(chunk))
	for i, k := range chunk {
		targets[i] = domain.RunTarget{KocID: k.KocID}
	}
	run, err = r.runs.Start(ctx, wf.By, n.CampaignName, n.Steps, targets, RunOptions{WorkflowID: wf.ID.Hex(), NodeID: tag})
	switch {
	case errors.Is(err, ErrBulkOff):
		return r.busy(t) // somebody turned the switch off again: it is turned on again at the next try
	case isBusy(err):
		return r.busy(t)
	case err != nil:
		log.Printf("workflow %s: cannot start the run of block %s: %v", wf.ID, tag, err)
		return advanceResult{stop: "send_failed"}
	}
	t.ChildID, t.Waits = run.ID.Hex(), 0
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeRunning, Count: len(t.Done)})
	return r.hold(t, r.tick)
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

	var again []domain.TokenKoc // not reached by the run: they go out in the next one
	for _, tg := range run.Targets {
		switch tg.Status {
		case domain.TargetSent, domain.TargetReplied:
			t.Done = append(t.Done, domain.TokenKoc{KocID: tg.KocID, SentAt: tg.StartedAt})
		case domain.TargetQueued, domain.TargetNotSent:
			again = append(again, domain.TokenKoc{KocID: tg.KocID})
		default: // skipped, failed, partial, unknown: nothing more is done for them, so nothing could be sent twice
			t.Skipped++
		}
	}
	rest := t.Kocs
	if len(run.Targets) <= len(rest) {
		rest = rest[len(run.Targets):]
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
