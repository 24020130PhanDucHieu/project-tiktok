package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// WorkflowKocs is what the stage block uses of the KOC service (*KocService).
type WorkflowKocs interface {
	FindActive(ctx context.Context, id string) (*domain.Koc, error) // a NotFound fault when there is no active KOC
	ApplyStage(ctx context.Context, koc *domain.Koc, p StageUpdate) (*StageResult, error)
}

// WithStages gives the runner the KOC service for the stage block.
func (r *WorkflowRunner) WithStages(k WorkflowKocs) *WorkflowRunner {
	r.kocs = k
	return r
}

func isFault(err error, kind domain.FaultKind) bool {
	var f *domain.Fault
	return errors.As(err, &f) && f.Kind == kind
}

// advanceStage moves every KOC of the token to the stage of the block, with the same action a booker uses (set_stage). A KOC
// that is gone, or that the KOC service refuses to change (blocked from contact), is skipped; a store that failed is read
// again a few times first. Setting the same stage again changes nothing, so a block that runs twice does no harm. The list
// goes on unchanged.
func (r *WorkflowRunner) advanceStage(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	if r.kocs == nil {
		return advanceResult{stop: "stage_unavailable"}
	}
	changed, skipped, failed := 0, 0, 0
	var changedIDs, notChangedIDs []string
	for _, k := range t.Kocs {
		koc, err := r.kocs.FindActive(ctx, k.KocID)
		switch {
		case isFault(err, domain.FaultNotFound):
			skipped++
			notChangedIDs = append(notChangedIDs, k.KocID)
			continue
		case err != nil:
			log.Printf("workflow %s: cannot read KOC %s: %v", wf.ID, k.KocID, err)
			failed++
			notChangedIDs = append(notChangedIDs, k.KocID)
			continue
		}
		to := n.Stage
		_, err = r.kocs.ApplyStage(ctx, koc, StageUpdate{Action: "set_stage", Stage: &to})
		switch {
		case isFault(err, domain.FaultConflict):
			skipped++
			notChangedIDs = append(notChangedIDs, k.KocID)
		case err != nil:
			log.Printf("workflow %s: cannot move KOC %s: %v", wf.ID, k.KocID, err)
			failed++
			notChangedIDs = append(notChangedIDs, k.KocID)
		default:
			changed++
			changedIDs = append(changedIDs, k.KocID)
		}
	}
	if failed > 0 && t.Reads < maxReplyReadRetries {
		t.Reads++
		return r.hold(t, time.Minute)
	}

	r.noteIDs(ctx, wf, n.ID, domain.EventDid, "", changedIDs)
	r.noteIDs(ctx, wf, n.ID, domain.EventSkip, "", notChangedIDs)

	var parts []string
	for _, p := range []struct {
		n    int
		text string
	}{{changed, "đã đổi"}, {skipped, "bỏ qua"}, {failed, "lỗi"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.text))
		}
	}
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Kocs), Reason: strings.Join(parts, ", ")})
	return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Kocs)}
}
