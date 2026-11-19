package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// maxBusyWaits is how many times a block waits (a minute each) for another job to finish before the workflow gives up.
const maxBusyWaits = 60

// isBusy tells that a store refused to start a job because another one is running. The service and the repository each have
// their own error value for it, so only the kind of the fault is the same.
func isBusy(err error) bool {
	var f *domain.Fault
	return errors.As(err, &f) && f.Kind == domain.FaultConflict
}

// hold keeps a token at its block for d more.
func (r *WorkflowRunner) hold(t domain.WorkflowToken, d time.Duration) advanceResult {
	t.ReadyAt = r.now().Add(d)
	return advanceResult{tokens: []domain.WorkflowToken{t}}
}

// advanceImport starts the import job of the block, follows it, and passes on the KOCs it added. A job the block started
// before (the server restarted) is found by its tags and followed; it is never started a second time.
func (r *WorkflowRunner) advanceImport(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	if t.ChildID == "" {
		job, err := r.imports.FindByWorkflow(ctx, wf.ID.Hex(), n.ID)
		if err != nil {
			log.Printf("workflow %s: cannot look for the import job of block %s: %v", wf.ID, n.ID, err)
			return r.hold(t, r.tick)
		}
		if job == nil {
			job, err = r.imports.Start(ctx, wf.By, wf.Source, wf.Rows, ImportOptions{WorkflowID: wf.ID.Hex(), NodeID: n.ID})
			switch {
			case isBusy(err):
				t.Waits++
				if t.Waits > maxBusyWaits {
					return advanceResult{stop: "import_busy"}
				}
				return r.hold(t, time.Minute)
			case err != nil:
				log.Printf("workflow %s: cannot start the import of block %s: %v", wf.ID, n.ID, err)
				return advanceResult{stop: "import_failed"}
			}
		}
		t.ChildID, t.Waits = job.ID.Hex(), 0
		return r.hold(t, r.tick)
	}

	job, err := r.imports.Find(ctx, t.ChildID)
	if err != nil {
		log.Printf("workflow %s: cannot read the import job %s: %v", wf.ID, t.ChildID, err)
		return r.hold(t, r.tick)
	}
	var kocs []domain.TokenKoc
	for _, row := range job.Rows {
		if row.KocID != "" {
			kocs = append(kocs, domain.TokenKoc{KocID: row.KocID})
		}
	}
	switch job.Status {
	case domain.ImportDone:
		st := domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(kocs)}
		if len(kocs) == 0 {
			st.Reason = "empty_list"
		}
		r.setState(ctx, wf, st)
		return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, kocs)}
	case domain.ImportStopped:
		return advanceResult{stop: "import_stopped:" + job.StopReason}
	}
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeRunning, Count: len(kocs)})
	return r.hold(t, r.tick)
}

// stopChildren asks the jobs the branches are waiting for to stop, when the workflow stops.
func (r *WorkflowRunner) stopChildren(ctx context.Context, wf *domain.Workflow) {
	for _, t := range wf.Tokens {
		if t.ChildID == "" {
			continue
		}
		n, ok := nodeOf(wf, t.NodeID)
		if !ok {
			continue
		}
		switch n.Type {
		case domain.NodeSend:
			// the switch is the stop button of the run that is running: turning it off ends the run
			if err := r.sw.SetEnabled(ctx, false, "workflow"); err != nil {
				log.Printf("workflow %s: cannot turn the switch off: %v", wf.ID, err)
			}
		case domain.NodeImport:
			if _, err := r.imports.RequestStop(ctx, t.ChildID); err != nil {
				log.Printf("workflow %s: cannot ask the import job %s to stop: %v", wf.ID, t.ChildID, err)
			}
		}
	}
}
