package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// AdvanceFollowUpStep runs reminder step 1..4 of the outreach chain and sends it: the sweep's path, which checks the
// automation switch first.
//
//	1 (+2 days): reminder #2 by email if available, otherwise Zalo
//	2 (+4 days): short Zalo reminder with an easy way out
//	3 (+7 days): phone-call task for the booker
//	4+        : chain exhausted -> DORMANT, reopen after 30 days
func (s *OutreachService) AdvanceFollowUpStep(ctx context.Context, koc *domain.Koc, step int) (*domain.Koc, error) {
	k, _, err := s.followUp(ctx, koc, step, true)
	return k, err
}

// PrepareFollowUp is AdvanceFollowUpStep for a booker's click: the same record-keeping, but the reminder comes back as
// a draft for the chat box instead of being sent.
func (s *OutreachService) PrepareFollowUp(ctx context.Context, koc *domain.Koc, step int) (*domain.Koc, string, error) {
	return s.followUp(ctx, koc, step, false)
}

func (s *OutreachService) followUp(ctx context.Context, koc *domain.Koc, step int, deliver bool) (*domain.Koc, string, error) {
	if koc.Dnc {
		return koc, "", nil
	}
	now := time.Now()
	draft := ""

	switch {
	case step == 1:
		draft = FollowUpDraft(koc, 1)
		if deliver {
			if koc.Email != "" {
				_ = s.mailer.SendKocEmail(koc.Email, koc.DisplayName, koc.TiktokID, draft) // SMTP may be unconfigured in dev
			}
			if _, err := s.DeliverMessageToKoc(ctx, koc, draft, true); err != nil {
				return nil, "", err
			}
		}
		koc.Stage, koc.ReminderStep, koc.LastContactedAt = domain.StageNoReply, 1, &now
	case step == 2:
		draft = FollowUpDraft(koc, 2)
		if deliver {
			if _, err := s.DeliverMessageToKoc(ctx, koc, draft, true); err != nil {
				return nil, "", err
			}
		}
		koc.Stage, koc.ReminderStep, koc.LastContactedAt = domain.StageNoReply, 2, &now
	case step == 3:
		// an instruction for the booker, not a message for the KOC: it is noted in the thread and never pushed to Zalo
		msg := fmt.Sprintf("[Task gọi điện Booker · +7 ngày] KOC chưa phản hồi sau 3 lần nhắn. Booker cần gọi điện trực tiếp tới SĐT %s.", orDefault(koc.Phone, "chưa cập nhật"))
		if err := s.recordInternalNote(ctx, koc, msg); err != nil {
			return nil, "", err
		}
		koc.Stage, koc.ReminderStep, koc.CallTaskPending, koc.TungKhongRep = domain.StageNoReply, 3, true, true
	case step >= 4:
		koc.Stage, koc.CallTaskPending, koc.TungKhongRep = domain.StageDormant, false, true
		koc.DormantUntil = domain.Ptr(time.Now().Add(30 * dayDuration))
	}
	return koc, draft, s.kocs.Save(ctx, koc)
}

// AdvanceVideoReminderStep runs video reminder 1..3 (+5/+8/+12 days after delivery) and sends it; step 4 flags the
// KOC STALLED. The sweep's path.
func (s *OutreachService) AdvanceVideoReminderStep(ctx context.Context, koc *domain.Koc, step int) (*domain.Koc, error) {
	k, _, err := s.videoReminder(ctx, koc, step, true)
	return k, err
}

// PrepareVideoReminder is AdvanceVideoReminderStep for a booker's click: the reminder comes back as a draft.
func (s *OutreachService) PrepareVideoReminder(ctx context.Context, koc *domain.Koc, step int) (*domain.Koc, string, error) {
	return s.videoReminder(ctx, koc, step, false)
}

func (s *OutreachService) videoReminder(ctx context.Context, koc *domain.Koc, step int, deliver bool) (*domain.Koc, string, error) {
	if koc.Dnc || koc.Stage == domain.StageVideoPosted || koc.Stage == domain.StageCompleted {
		return koc, "", nil
	}
	now := time.Now()
	text := VideoReminderDraft(koc, step)
	switch {
	case step >= 1 && step <= 3:
		if deliver {
			if _, err := s.DeliverMessageToKoc(ctx, koc, text, true); err != nil {
				return nil, "", err
			}
		}
		koc.VideoReminderStep, koc.LastContactedAt = step, &now
	case step >= 4:
		koc.Stage, koc.StalledFlag, koc.KocVideoReply = domain.StageStalled, true, "SILENT"
	}
	return koc, text, s.kocs.Save(ctx, koc)
}

// ShipmentSync is the result of syncing a J&T shipment.
type ShipmentSync struct {
	Koc      *domain.Koc
	Tracking *JTTracking
	// JustDelivered is true when this sync is the one that found the parcel delivered. Telling the creator is left to
	// the caller: the sweep does it when the automation switch is on; a booker is offered DeliveredDraft instead.
	JustDelivered bool
}

// SyncKocShipment queries J&T, updates delivery status and flags shipments in transit for more than 5 days.
func (s *OutreachService) SyncKocShipment(ctx context.Context, koc *domain.Koc) (*ShipmentSync, error) {
	if koc.TrackingCode == "" {
		return &ShipmentSync{Koc: koc}, nil
	}
	now := time.Now()
	phone4 := ResolvePhoneLast4(koc)
	if phone4 == "" {
		koc.TrackingStatusText = "Cần 4 số cuối SĐT người nhận để theo dõi vận đơn J&T"
		return &ShipmentSync{Koc: koc}, nil
	}
	tracking, err := s.tracker.QueryTracking(ctx, koc.TrackingCode, phone4)
	if err != nil {
		return nil, err
	}
	koc.LastTrackedAt = &now
	koc.TrackingStatusText = tracking.StatusText

	shipped := now
	if koc.ShippedAt != nil {
		shipped = *koc.ShippedAt
	}
	daysInTransit := now.Sub(shipped).Hours() / 24

	justDelivered := false
	if tracking.Delivered {
		justDelivered = koc.Stage != domain.StageDelivered
		koc.Stage = domain.StageDelivered
		if koc.DeliveredAt == nil {
			koc.DeliveredAt = &now
		}
		koc.ShippingDelayedAlert = false
		koc.RaiseCollabStep(3)
	} else if daysInTransit > 5 {
		koc.ShippingDelayedAlert = true
	}
	if err := s.kocs.Save(ctx, koc); err != nil {
		return nil, err
	}
	return &ShipmentSync{Koc: koc, Tracking: tracking, JustDelivered: justDelivered}, nil
}

// ChannelSync is the result of syncing a TikTok channel.
type ChannelSync struct {
	Koc         *domain.Koc
	ChannelData *TikTokChannel
}

// SyncKocTikTokChannel refreshes follower/avatar/videos and (optionally) auto-detects the collab video.
func (s *OutreachService) SyncKocTikTokChannel(ctx context.Context, koc *domain.Koc, autoDetectCollabVideo bool) (*ChannelSync, error) {
	data, err := s.tiktok.FetchChannel(ctx, koc.TiktokID)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	if data.Found {
		p := data.Profile
		if p.Followers > 0 {
			koc.Followers = p.Followers
		}
		if p.AvatarURL != "" {
			koc.AvatarURL = p.AvatarURL
		}
		if p.DisplayName != "" && (koc.DisplayName == "" || strings.HasPrefix(koc.DisplayName, "KOC @")) {
			koc.DisplayName = p.DisplayName
		}
		if koc.Phone == "" && p.Phone != "" {
			koc.Phone = p.Phone
		}
		if koc.Email == "" && p.Email != "" {
			koc.Email = p.Email
		}
	}
	if len(data.Videos) > 0 {
		koc.Videos = data.Videos
	}
	koc.ChannelSyncedAt = &now

	// A new video matching the product (or simply the latest one) means the collab video went live.
	if autoDetectCollabVideo && len(koc.Videos) > 0 && koc.VideoURL == "" {
		var keywords []string
		for _, w := range strings.Fields(strings.ToLower(koc.ProductName)) {
			if len([]rune(w)) >= 3 {
				keywords = append(keywords, w)
			}
		}
		matched := koc.Videos[0]
	search:
		for _, v := range koc.Videos {
			title := strings.ToLower(v.Title)
			for _, kw := range keywords {
				if strings.Contains(title, kw) {
					matched = v
					break search
				}
			}
		}
		koc.VideoURL = matched.URL
		koc.VideoPostedAt = &now
		if views := ParseCompactCount(matched.Views); views > 0 {
			koc.VideoViews = views
		}
		koc.Stage = domain.StageVideoPosted
		koc.RaiseCollabStep(4)
		koc.StalledFlag = false
		koc.CallTaskPending = false
	}
	if err := s.kocs.Save(ctx, koc); err != nil {
		return nil, err
	}
	return &ChannelSync{Koc: koc, ChannelData: data}, nil
}

// FetchVideoMetrics validates a TikTok video URL.
func (s *OutreachService) FetchVideoMetrics(ctx context.Context, videoURL string) (*TikTokVideoMetrics, error) {
	return s.tiktok.FetchVideoMetrics(ctx, videoURL)
}

// FetchChannel looks up a TikTok channel.
func (s *OutreachService) FetchChannel(ctx context.Context, handle string) (*TikTokChannel, error) {
	return s.tiktok.FetchChannel(ctx, handle)
}

// QueryTracking looks up a J&T bill code.
func (s *OutreachService) QueryTracking(ctx context.Context, code, phoneLast4 string) (*JTTracking, error) {
	return s.tracker.QueryTracking(ctx, code, phoneLast4)
}

// SendEmail emails a KOC.
func (s *OutreachService) SendEmail(koc *domain.Koc, text string) error {
	return s.mailer.SendKocEmail(koc.Email, koc.DisplayName, koc.TiktokID, text)
}

// ResolvePhoneLast4 picks the J&T verification digits: the saved value first, then the KOC's phone.
func ResolvePhoneLast4(koc *domain.Koc) string {
	if v := NormalizePhoneLast4(koc.TrackingPhoneLast4); v != "" {
		return v
	}
	return NormalizePhoneLast4(koc.Phone)
}
