package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ImageLoader reads a stored campaign picture (*repository.CampaignRepository).
type ImageLoader interface {
	Image(ctx context.Context, id string) (*domain.CampaignImage, error)
}

// SequenceHooks lets the caller pace and record a chain of steps.
type SequenceHooks struct {
	// Pause is called before every step (a human-looking wait). An error stops the chain.
	Pause func(ctx context.Context) error
	// Progress stores the state of the target. It is called BEFORE a step goes out (status "sending") and after it.
	Progress func(t domain.RunTarget) error
}

// SendSequence sends the steps to one KOC, one after the other, and returns how it went. Rules, in order:
//   - a KOC that is missing, has no phone or asked not to be contacted is skipped;
//   - the KOC is claimed in the store once for the whole chain (the 24 hour rule);
//   - before every step the KOC is checked for a reply: an answer ends the chain;
//   - the target is stored as "sending" before Zalo is called, and a step whose outcome cannot be known (the bridge did
//     not answer) is "unknown" and is never tried again.
//
// zaloFault is true when the chain ended because of Zalo (a refusal, no answer, a line that is down).
func (s *OutreachService) SendSequence(ctx context.Context, koc *domain.Koc, steps []domain.CampaignStep, images ImageLoader, t domain.RunTarget, h SequenceHooks) (out domain.RunTarget, zaloFault bool) {
	out = t
	end := func(status, reason string) (domain.RunTarget, bool) {
		out.Status, out.Reason = status, reason
		return out, false
	}
	progress := func() error {
		if h.Progress != nil {
			return h.Progress(out)
		}
		return nil
	}

	if koc == nil {
		return end(domain.TargetSkipped, "not_found")
	}
	if NormalizePhone(koc.Phone) == "" {
		return end(domain.TargetSkipped, "no_phone")
	}
	if koc.Dnc || koc.DaHopTac {
		return end(domain.TargetSkipped, "dnc_or_blocked")
	}

	now := s.now()
	claimed, previous, err := s.kocs.ClaimContact(ctx, koc.ID, now.Add(-dayDuration), now)
	if err != nil {
		return end(domain.TargetFailed, err.Error())
	}
	if !claimed {
		return end(domain.TargetSkipped, "already_sent_within_24h")
	}
	release := func() { _ = s.kocs.ReleaseContact(ctx, koc.ID, now, previous) } // a KOC nothing reached is not "contacted"

	preview := ""
	if len(steps) > 0 && steps[0].Kind == domain.StepText {
		preview = RenderTemplate(steps[0].Text, koc)
	}
	conv, status, err := s.openThread(ctx, koc, preview, true)
	if err != nil {
		release()
		return end(domain.TargetFailed, err.Error())
	}
	switch status {
	case DeliveryNotOnZalo:
		release()
		return end(domain.TargetSkipped, "not_on_zalo")
	case DeliveryZaloNotReady:
		release()
		out.Status, out.Reason = domain.TargetNotSent, "zalo_not_ready"
		return out, true
	}

	out.StartedAt = &now
	lastText := ""
	notRecorded := false
	// fail ends the chain on a step Zalo did not take; the claim goes back only when nothing was sent to this KOC
	fail := func(status, reason string, fault bool) (domain.RunTarget, bool) {
		if out.StepsSent == 0 && status != domain.TargetUnknown {
			release()
			if status == domain.TargetPartial { // nothing was sent: "partial" would say otherwise
				status = domain.TargetNotSent
			}
		}
		out.Status, out.Reason = status, reason
		progress()
		return out, fault
	}

	for _, step := range steps {
		if h.Pause != nil {
			if err := h.Pause(ctx); err != nil {
				if out.StepsSent == 0 {
					release()
					out.Status, out.Reason = domain.TargetNotSent, "stopped"
				} else {
					out.Status, out.Reason = domain.TargetPartial, "stopped"
				}
				progress()
				return out, false
			}
		}
		if replied, err := s.msgs.InboundSince(ctx, conv.ID, now); err != nil {
			return fail(domain.TargetPartial, "reply_check_failed", false)
		} else if replied {
			out.Status, out.Reason = domain.TargetReplied, ""
			progress()
			return out, false
		}

		var (
			stored string
			res    ZaloDelivery
			sendEr error
		)
		switch step.Kind {
		case domain.StepImage:
			img, err := images.Image(ctx, step.ImageID.Hex())
			if err != nil || img == nil {
				return fail(domain.TargetFailed, "image_missing", false)
			}
			caption := strings.TrimSpace(RenderTemplate(step.Caption, koc))
			stored = strings.TrimSpace("[Ảnh] " + caption)
			out.Status = domain.TargetSending
			if err := progress(); err != nil {
				return fail(domain.TargetPartial, "store_unavailable", false)
			}
			res, sendEr = s.zalo.SendImage(ctx, conv.ExternalContactID, ZaloImage{Filename: img.Filename, Data: img.Data, Width: img.Width, Height: img.Height, Caption: caption})
		default:
			stored = RenderTemplate(step.Text, koc)
			out.Status = domain.TargetSending
			if err := progress(); err != nil {
				return fail(domain.TargetPartial, "store_unavailable", false)
			}
			res, sendEr = s.zalo.SendPersonalText(ctx, conv.ExternalContactID, stored)
		}

		if sendEr != nil && (errors.Is(sendEr, ErrZaloUnreachable) || ctx.Err() != nil) {
			return fail(domain.TargetUnknown, "step_outcome_unknown", true)
		}
		if sendEr != nil {
			return fail(domain.TargetFailed, sendEr.Error(), true)
		}
		if !res.Delivered {
			return fail(domain.TargetFailed, "zalo_not_delivered", true)
		}

		out.StepsSent++
		lastText = stored
		if err := s.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Text: stored, Status: domain.MessageSent}); err != nil {
			notRecorded = true // it went out: the chain goes on, and the report says it was not stored
		}
		progress()
	}

	koc.LastContactedAt = &now
	if lastText != "" {
		_ = s.convs.SetLastMessageAt(ctx, conv.ID, lastText, s.now())
	}
	out.Status, out.Reason = domain.TargetSent, ""
	if notRecorded {
		out.Reason = "message_not_recorded"
	}
	progress()
	return out, false
}
