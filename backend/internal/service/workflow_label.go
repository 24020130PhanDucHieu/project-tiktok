package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// WorkflowLabels is what the label block reads and writes (the KOC store and the conversation store).
type WorkflowLabels interface {
	FindActiveByID(ctx context.Context, id string) (*domain.Koc, error)
	FindByHandle(ctx context.Context, handle string) (*domain.Conversation, error)
	// AddLabel puts a label on a conversation without touching the others (a set: adding twice changes nothing).
	AddLabel(ctx context.Context, convID domain.ID, labelID string) error
}

// WithLabeler gives the runner the stores of the label block.
func (r *WorkflowRunner) WithLabeler(l WorkflowLabels) *WorkflowRunner {
	r.labels = l
	return r
}

// advanceLabel puts the label of the block on the conversation of every KOC of the token. A KOC that is gone or has no
// conversation is skipped; a store that failed is read again a few times (a minute apart, and the work is safe to repeat)
// before the KOCs it failed for are given up. The list goes on unchanged: a label block only marks.
func (r *WorkflowRunner) advanceLabel(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	if r.labels == nil {
		return advanceResult{stop: "label_unavailable"}
	}
	labeled, noThread, failed := 0, 0, 0
	var labeledIDs, notLabeledIDs []string
	for _, k := range t.Kocs {
		koc, err := r.labels.FindActiveByID(ctx, k.KocID)
		if err != nil {
			log.Printf("workflow %s: cannot read KOC %s: %v", wf.ID, k.KocID, err)
			failed++
			notLabeledIDs = append(notLabeledIDs, k.KocID)
			continue
		}
		if koc == nil {
			noThread++
			notLabeledIDs = append(notLabeledIDs, k.KocID)
			continue
		}
		conv, err := r.labels.FindByHandle(ctx, "@"+koc.TiktokID)
		if err != nil {
			log.Printf("workflow %s: cannot read the conversation of KOC %s: %v", wf.ID, k.KocID, err)
			failed++
			notLabeledIDs = append(notLabeledIDs, k.KocID)
			continue
		}
		if conv == nil {
			noThread++
			notLabeledIDs = append(notLabeledIDs, k.KocID)
			continue
		}
		if err := r.labels.AddLabel(ctx, conv.ID, n.LabelID); err != nil {
			log.Printf("workflow %s: cannot label the conversation of KOC %s: %v", wf.ID, k.KocID, err)
			failed++
			notLabeledIDs = append(notLabeledIDs, k.KocID)
			continue
		}
		labeled++
		labeledIDs = append(labeledIDs, k.KocID)
	}
	if failed > 0 && t.Reads < maxReplyReadRetries {
		t.Reads++
		return r.hold(t, time.Minute)
	}

	r.noteIDs(ctx, wf, n.ID, domain.EventDid, "", labeledIDs)
	r.noteIDs(ctx, wf, n.ID, domain.EventSkip, "", notLabeledIDs)

	var parts []string
	for _, p := range []struct {
		n    int
		text string
	}{{labeled, "đã gắn nhãn"}, {noThread, "không có cuộc trò chuyện"}, {failed, "lỗi"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.text))
		}
	}
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Kocs), Reason: strings.Join(parts, ", ")})
	t.Reads = 0
	return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Kocs)}
}
