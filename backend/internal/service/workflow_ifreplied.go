package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// maxReplyReadRetries is how many times a block reads again (a minute apart) when a store failed, before the KOCs it could
// not read are given up.
const maxReplyReadRetries = 3

// advanceIfReplied splits the KOCs of the token into the ones that answered after their steps went out and the ones that
// did not. A KOC that cannot be told (no send time, no thread, a store that keeps failing) goes to NEITHER branch: it is
// better to leave one KOC without a reminder than to remind one who already answered.
func (r *WorkflowRunner) advanceIfReplied(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	var replied, unanswered []domain.TokenKoc
	unknown, failures := 0, 0
	for _, k := range t.Kocs {
		if k.SentAt == nil {
			unknown++
			continue
		}
		did, known, err := r.replies.RepliedSince(ctx, k.KocID, *k.SentAt)
		switch {
		case err != nil:
			log.Printf("workflow %s: cannot tell whether KOC %s answered: %v", wf.ID, k.KocID, err)
			failures++
			unknown++
		case !known:
			unknown++
		case did:
			replied = append(replied, k)
		default:
			unanswered = append(unanswered, k)
		}
	}
	if failures > 0 && t.Waits < maxReplyReadRetries { // a store that blinked: read again before giving anybody up
		t.Waits++
		return r.hold(t, time.Minute)
	}

	var parts []string
	for _, p := range []struct {
		n    int
		text string
	}{{len(replied), "đã trả lời"}, {len(unanswered), "chưa trả lời"}, {unknown, "không rõ"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.text))
		}
	}
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Kocs), Reason: strings.Join(parts, ", ")})

	tokens := r.leave(ctx, wf, n.ID, domain.PortReplied, replied)
	return advanceResult{tokens: append(tokens, r.leave(ctx, wf, n.ID, domain.PortUnanswered, unanswered)...)}
}
