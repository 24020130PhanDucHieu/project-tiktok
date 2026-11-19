package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// MaxEmailsPerBlock is how many emails one email block sends at most.
const MaxEmailsPerBlock = 100

// WorkflowEmailer sends one email to one KOC (*EmailSender).
type WorkflowEmailer interface {
	SendEmailTo(ctx context.Context, kocID, subject, text string) (EmailStatus, error)
}

// WithEmailer gives the runner the sender of the email block.
func (r *WorkflowRunner) WithEmailer(e WorkflowEmailer) *WorkflowRunner {
	r.emailer = e
	return r
}

// advanceEmail sends the emails of the block, ONE KOC at each look, with a pause of 3 to 8 seconds after every email, only inside
// the send window (08:30 to 20:30), and at most MaxEmailsPerBlock. A KOC that answered since it was last sent to gets
// nothing, and neither does one whose answer cannot be checked. Two failures in a row stop the workflow; so does an SMTP that is
// not configured. The sender marks a KOC as contacted before it sends, so when the block runs again after a restart a KOC
// that was already emailed is skipped, never emailed twice. The KOCs that got the email go on.
func (r *WorkflowRunner) advanceEmail(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	if r.emailer == nil {
		return advanceResult{stop: "email_unavailable"}
	}
	if len(t.Kocs) == 0 || len(t.Done) >= MaxEmailsPerBlock {
		return r.finishEmail(ctx, wf, n, t)
	}
	now := r.now()
	if !IsWithinAllowedSendWindow(now) {
		return r.hold(t, nextSendWindowStart(now).Sub(now))
	}
	k, rest := t.Kocs[0], t.Kocs[1:]

	if k.SentAt != nil && r.replies != nil {
		did, known, err := r.replies.RepliedSince(ctx, k.KocID, *k.SentAt)
		switch {
		case err != nil && t.Reads < maxReplyReadRetries: // a store that blinked: look again at the next tick
			t.Reads++
			return r.hold(t, r.tick)
		case err != nil || !known || did:
			t.Kocs, t.Skipped, t.Reads = rest, t.Skipped+1, 0
			return advanceResult{tokens: []domain.WorkflowToken{t}}
		}
	}
	t.Reads = 0

	status, err := r.emailer.SendEmailTo(ctx, k.KocID, n.Subject, n.Text)
	switch {
	case errors.Is(err, ErrEmailNotConfigured):
		return advanceResult{stop: "email_not_configured"}
	case err != nil:
		t.Waits++
		t.Kocs, t.Skipped = rest, t.Skipped+1
		if t.Waits >= 2 {
			return advanceResult{stop: "email_errors"}
		}
		return advanceResult{tokens: []domain.WorkflowToken{t}}
	}
	t.Waits, t.Kocs = 0, rest
	if status != EmailSent {
		t.Skipped++
		return advanceResult{tokens: []domain.WorkflowToken{t}}
	}
	sentAt := now
	t.Done = append(t.Done, domain.TokenKoc{KocID: k.KocID, SentAt: &sentAt})
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeRunning, Count: len(t.Done)})
	return r.hold(t, r.emailPause())
}

// finishEmail passes on the KOCs that got the email.
func (r *WorkflowRunner) finishEmail(ctx context.Context, wf *domain.Workflow, n domain.WorkflowNode, t domain.WorkflowToken) advanceResult {
	var parts []string
	if t.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d bỏ qua", t.Skipped))
	}
	if len(t.Kocs) > 0 {
		parts = append(parts, fmt.Sprintf("đủ %d email, %d KOC chưa gửi", MaxEmailsPerBlock, len(t.Kocs)))
	}
	r.setState(ctx, wf, domain.NodeState{NodeID: n.ID, Status: domain.NodeDone, Count: len(t.Done), Reason: strings.Join(parts, ", ")})
	return advanceResult{tokens: r.leave(ctx, wf, n.ID, domain.PortOut, t.Done)}
}

func defaultEmailPause() time.Duration { return randomBetween(3*time.Second, 8*time.Second) }
