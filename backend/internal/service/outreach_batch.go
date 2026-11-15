package service

import (
	"context"
	"strings"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Status of one KOC in a template batch (POST /kocs/message-batch).
const (
	BatchSent    = "sent"     // the message went out over Zalo
	BatchPending = "pending"  // stored, but Zalo did not take it
	BatchSkipped = "skipped"  // not eligible; Reason says why
	BatchNotSent = "not_sent" // not tried, because the batch stopped; Reason says why
	BatchFailed  = "failed"   // the server hit an error for this KOC
)

// BatchTarget is one id of a batch with the active KOC found for it; Koc is nil when there is none.
type BatchTarget struct {
	ID  string
	Koc *domain.Koc
}

// BatchResult is what happened to one KOC of a batch.
type BatchResult struct {
	KocID  string `json:"kocId"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// BatchReport is the answer of a batch. Stop tells the caller not to send the next batch.
type BatchReport struct {
	Results    []BatchResult `json:"results"`
	Stop       bool          `json:"stop"`
	StopReason string        `json:"stopReason,omitempty"`
}

// RenderTemplate fills {ten} with the last word of the KOC's name, or "bạn" when the KOC has no name.
func RenderTemplate(body string, koc *domain.Koc) string {
	name := "bạn"
	if words := strings.Fields(koc.DisplayName); len(words) > 0 {
		name = words[len(words)-1]
	}
	return strings.ReplaceAll(body, "{ten}", name)
}

// SendTemplateBatch sends body (with {ten} filled per KOC) to each target, in order, through the same anti-spam guard
// as the other proactive messages. It refuses the whole batch with ErrAutomationOff while the master switch is off.
// A guard verdict about the day or the hour (quota, send window), a Zalo line that cannot carry the message, or a
// failure to record the contact stops the batch: the KOCs not reached are reported as not_sent and Stop is set.
func (s *OutreachService) SendTemplateBatch(ctx context.Context, body string, targets []BatchTarget) (*BatchReport, error) {
	if !s.automationOn(ctx) {
		return nil, ErrAutomationOff
	}
	report := &BatchReport{Results: make([]BatchResult, 0, len(targets))}
	add := func(id, status, reason string) {
		report.Results = append(report.Results, BatchResult{KocID: id, Status: status, Reason: reason})
	}
	// stopFrom reports targets[from:] as not sent and ends the batch.
	stopFrom := func(from int, reason string) {
		for _, t := range targets[from:] {
			add(t.ID, BatchNotSent, reason)
		}
		report.Stop, report.StopReason = true, reason
	}

	for i, t := range targets {
		koc := t.Koc
		if koc == nil {
			add(t.ID, BatchSkipped, "not_found")
			continue
		}
		if NormalizePhone(koc.Phone) == "" {
			add(t.ID, BatchSkipped, "no_phone")
			continue
		}
		if check := s.CanSendProactiveMessage(koc, false); !check.Allowed {
			if check.Reason == "daily_quota_reached" || check.Reason == "outside_0830_2030_window" {
				stopFrom(i, check.Reason)
				return report, nil
			}
			add(t.ID, BatchSkipped, check.Reason)
			continue
		}

		// Claim the KOC in the store before anything is sent: the copy loaded earlier may be out of date (another sender,
		// a sweep), and only the store can say, in one step, that nobody contacted the KOC in the last 24 hours.
		now := s.now()
		claimed, previous, err := s.kocs.ClaimContact(ctx, koc.ID, now.Add(-dayDuration), now)
		if err != nil {
			add(t.ID, BatchFailed, err.Error())
			continue
		}
		if !claimed {
			add(t.ID, BatchSkipped, "already_sent_within_24h")
			continue
		}
		release := func() { _ = s.kocs.ReleaseContact(ctx, koc.ID, now, previous) } // a KOC nothing reached is not "contacted"

		d, err := s.deliverToKoc(ctx, koc, RenderTemplate(body, koc), true, true)
		if err != nil && (d == nil || d.Status != domain.MessageSent) {
			release()
			add(t.ID, BatchFailed, err.Error())
			continue
		}
		if err != nil { // Zalo took the message but it could not be stored: it went out, so the claim stays
			koc.LastContactedAt = &now
			add(t.ID, BatchSent, "message_not_recorded")
			stopFrom(i+1, "message_not_recorded")
			return report, nil
		}
		switch d.Status {
		case DeliveryNotOnZalo:
			release()
			add(t.ID, BatchSkipped, "not_on_zalo")
		case DeliveryZaloNotReady:
			release()
			stopFrom(i, "zalo_not_ready")
			return report, nil
		case "blocked_dnc":
			release()
			add(t.ID, BatchSkipped, "dnc_or_blocked")
		case domain.MessagePending:
			release()
			add(t.ID, BatchPending, "")
			stopFrom(i+1, "zalo_not_ready")
			return report, nil
		case domain.MessageSent:
			koc.LastContactedAt = &now
			add(t.ID, BatchSent, "")
		default:
			release()
			add(t.ID, BatchFailed, "unexpected delivery status "+d.Status)
		}
	}
	return report, nil
}
