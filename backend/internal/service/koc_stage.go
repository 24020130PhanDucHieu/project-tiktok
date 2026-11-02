package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
)

// StageUpdate is the PATCH /kocs/:id/stage payload. Pointer fields distinguish "absent" from zero values.
type StageUpdate struct {
	Action string `json:"action" validate:"required,oneof=qualify approve_cast filter_out toggle_collaborated toggle_dnc send_invite remind_step mark_replied mark_dormant reopen_dormant agree decline ship_sample mark_delivered post_video remind_video video_reply return_product complete_returned complete_collab set_stage"`

	ContentPillar      *string  `json:"contentPillar" validate:"omitempty,max=150"`
	TargetAudience     *string  `json:"targetAudience" validate:"omitempty,max=150"`
	Campaign           *string  `json:"campaign" validate:"omitempty,max=120"`
	ProductName        *string  `json:"productName" validate:"omitempty,max=150"`
	CommissionRate     *float64 `json:"commissionRate" validate:"omitempty,gte=0,lte=100"`
	CastFee            *float64 `json:"castFee" validate:"omitempty,gte=0"`
	RejectCode         *string  `json:"rejectCode" validate:"omitempty,max=50"`
	RejectNote         *string  `json:"rejectNote" validate:"omitempty,max=300"`
	DeclineCode        *string  `json:"declineCode" validate:"omitempty,max=50"`
	DeclineNote        *string  `json:"declineNote" validate:"omitempty,max=300"`
	InviteBrief        *string  `json:"inviteBrief" validate:"omitempty,max=1000"`
	ReminderStep       *int     `json:"reminderStep" validate:"omitempty,min=1,max=3"`
	InboundText        *string  `json:"inboundText" validate:"omitempty,max=2000"`
	Recipient          *string  `json:"recipient" validate:"omitempty,max=120"`
	ShippingAddress    *string  `json:"shippingAddress" validate:"omitempty,max=300"`
	Phone              *string  `json:"phone" validate:"omitempty,max=30"`
	ShippingCarrier    *string  `json:"shippingCarrier"`
	TrackingCode       *string  `json:"trackingCode" validate:"omitempty,max=100"`
	TrackingPhoneLast4 *string  `json:"trackingPhoneLast4" validate:"omitempty,len=4,numeric"`
	ShippedAt          *string  `json:"shippedAt" validate:"omitempty,max=50"`
	VideoURL           *string  `json:"videoUrl" validate:"omitempty,max=500"`
	VideoReminderStep  *int     `json:"videoReminderStep" validate:"omitempty,min=1,max=4"`
	KocVideoReply      *string  `json:"kocVideoReply"`
	VideoViews         *float64 `json:"videoViews" validate:"omitempty,gte=0"`
	VideoOrders        *float64 `json:"videoOrders" validate:"omitempty,gte=0"`
	VideoGmv           *float64 `json:"videoGmv" validate:"omitempty,gte=0"`
	PostReceiveCode    *string  `json:"postReceiveCode" validate:"omitempty,max=50"`
	PostReceiveNote    *string  `json:"postReceiveNote" validate:"omitempty,max=300"`
	ReturnTrackingCode *string  `json:"returnTrackingCode" validate:"omitempty,max=100"`
	ReturnCondition    *string  `json:"returnCondition" validate:"omitempty,max=200"`
	MarkDaHopTac       *bool    `json:"markDaHopTac"`
	DaHopTac           *bool    `json:"daHopTac"`
	Dnc                *bool    `json:"dnc"`
	Stage              *string  `json:"stage" validate:"omitempty,oneof=SOURCED QUALIFIED AWAITING_APPROVAL FILTERED_OUT CONTACTED NO_REPLY REPLIED DORMANT AGREED DECLINED SHIPPING DELIVERED VIDEO_POSTED NOT_FIT RETURNING RETURNED STALLED COMPLETED"`
}

func str(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

func setStr(dst *string, p *string) {
	if p != nil {
		*dst = *p
	}
}

func setNum(dst *float64, p *float64) {
	if p != nil {
		*dst = *p
	}
}

func castTypeFor(fee float64) string {
	if fee > 0 {
		return "cast"
	}
	return "freecast"
}

// ApplyStageUpdate performs one booking action on a KOC and persists it.
func (s *KocService) ApplyStageUpdate(ctx context.Context, koc *domain.Koc, p StageUpdate) (*domain.Koc, error) {
	if koc.DaHopTac && p.Action != "toggle_collaborated" {
		return nil, httpx.NewError(409, "KOC này đang bị chặn liên hệ (da_hop_tac = true)")
	}
	if (p.ShippingCarrier != nil && !oneOf(*p.ShippingCarrier, "", "J&T Express", "Tự vận chuyển")) ||
		(p.KocVideoReply != nil && !oneOf(*p.KocVideoReply, "", "WILL_POST", "NOT_FIT", "SILENT")) {
		return nil, httpx.NewError(400, "Dữ liệu không hợp lệ")
	}
	now := time.Now()

	switch p.Action {
	// ---- Stage 1 & collab steps 1-2 ----
	case "qualify":
		castFee := koc.CastFee
		if p.CastFee != nil {
			castFee = *p.CastFee
		}
		setStr(&koc.ContentPillar, p.ContentPillar)
		setStr(&koc.TargetAudience, p.TargetAudience)
		setStr(&koc.Campaign, p.Campaign)
		setStr(&koc.ProductName, p.ProductName)
		setNum(&koc.CommissionRate, p.CommissionRate)
		koc.CastFee, koc.CastType = castFee, castTypeFor(castFee)
		koc.Stage = domain.StageQualified
		if castFee > 0 {
			koc.Stage = domain.StageAwaitingApproval
		}
		koc.RaiseCollabStep(2)
		koc.RejectCode, koc.RejectNote = "", ""
	case "approve_cast":
		koc.Stage = domain.StageQualified
		if p.CastFee != nil {
			koc.CastFee = *p.CastFee
			koc.CastType = castTypeFor(koc.CastFee)
		}
		koc.RaiseCollabStep(1)
	case "filter_out":
		koc.Stage = domain.StageFilteredOut
		koc.RejectCode = orDefault(str(p.RejectCode, ""), "F01")
		koc.RejectNote = str(p.RejectNote, "")
	case "toggle_collaborated":
		koc.DaHopTac = p.DaHopTac != nil && *p.DaHopTac
	case "toggle_dnc":
		koc.Dnc = p.Dnc != nil && *p.Dnc

	// ---- Stage 2 · outreach & follow-up ----
	case "send_invite":
		return s.outreach.SendFirstInvite(ctx, koc, str(p.InviteBrief, ""), true)
	case "remind_step":
		step := koc.ReminderStep + 1
		if step > 3 {
			step = 3
		}
		if p.ReminderStep != nil && *p.ReminderStep != 0 {
			step = *p.ReminderStep
		}
		return s.outreach.AdvanceFollowUpStep(ctx, koc, step)
	case "mark_replied":
		koc.Stage = domain.StageReplied
		koc.LastRepliedAt = &now
		koc.CallTaskPending = false
		if p.InboundText != nil && *p.InboundText != "" {
			parsed := ParseKocInboundText(*p.InboundText)
			if parsed.Phone != "" && koc.Phone == "" {
				koc.Phone = parsed.Phone
			}
			if parsed.ShippingAddress != "" {
				koc.ShippingAddress = parsed.ShippingAddress
				koc.Stage = domain.StageAgreed
				koc.RaiseCollabStep(2)
			}
		}
	case "mark_dormant":
		koc.Stage, koc.CallTaskPending = domain.StageDormant, false
		koc.DormantUntil = domain.Ptr(now.Add(30 * dayDuration))
	case "reopen_dormant":
		koc.Stage, koc.ReminderStep, koc.CallTaskPending, koc.DormantUntil = domain.StageQualified, 0, false, nil
	case "agree":
		koc.Stage, koc.CallTaskPending = domain.StageAgreed, false
		koc.DeclineCode, koc.DeclineNote = "", ""
		setStr(&koc.ShippingAddress, p.ShippingAddress)
		setStr(&koc.Recipient, p.Recipient)
		if p.Phone != nil {
			koc.Phone = NormalizePhone(*p.Phone)
		}
		koc.RaiseCollabStep(2)
	case "decline":
		koc.Stage, koc.CallTaskPending = domain.StageDeclined, false
		koc.DeclineCode = orDefault(str(p.DeclineCode, ""), "R01")
		koc.DeclineNote = str(p.DeclineNote, "")

	// ---- Stage 3 · shipping (J&T Express / self-ship) ----
	case "ship_sample":
		setStr(&koc.Recipient, p.Recipient)
		if p.Phone != nil {
			koc.Phone = NormalizePhone(*p.Phone)
		}
		setStr(&koc.ShippingAddress, p.ShippingAddress)
		koc.ShippingCarrier = orDefault(str(p.ShippingCarrier, ""), "J&T Express")
		koc.TrackingCode = strings.TrimSpace(str(p.TrackingCode, ""))
		setStr(&koc.TrackingPhoneLast4, p.TrackingPhoneLast4)
		koc.ShippedAt = &now
		if p.ShippedAt != nil && *p.ShippedAt != "" {
			if t, ok := parseClientTime(*p.ShippedAt); ok {
				koc.ShippedAt = &t
			}
		}
		koc.Stage = domain.StageShipping
		koc.RaiseCollabStep(3)
		koc.ShippingDelayedAlert = false

		if koc.ShippingCarrier == "J&T Express" && koc.TrackingCode != "" {
			if _, err := s.outreach.SyncKocShipment(ctx, koc); err != nil {
				return nil, err
			}
		} else {
			koc.TrackingStatusText = fmt.Sprintf("Đang vận chuyển (%s)", koc.ShippingCarrier)
		}
		tracking := ""
		if koc.TrackingCode != "" {
			tracking = fmt.Sprintf(" (Mã vận đơn: #%s)", koc.TrackingCode)
		}
		msg := fmt.Sprintf("[Gửi mẫu thành công] Bên mình đã gửi sản phẩm mẫu tới địa chỉ \"%s\" qua %s%s. Bạn để ý điện thoại nhận hàng giúp mình nhé!",
			orDefault(koc.ShippingAddress, "của bạn"), koc.ShippingCarrier, tracking)
		if _, err := s.outreach.DeliverMessageToKoc(ctx, koc, msg, true); err != nil {
			return nil, err
		}
	case "mark_delivered":
		koc.Stage, koc.DeliveredAt, koc.ShippingDelayedAlert = domain.StageDelivered, &now, false
		koc.TrackingStatusText = "Đã giao hàng thành công (DELIVERED)"
		koc.RaiseCollabStep(3)
		if _, err := s.outreach.DeliverMessageToKoc(ctx, koc,
			"[Đã giao mẫu] Đơn hàng mẫu đã giao thành công! Bạn trải nghiệm sản phẩm và lên video theo brief giúp nhãn hàng nhé.", true); err != nil {
			return nil, err
		}

	// ---- Stage 4 · video, returns & performance ----
	case "post_video":
		if url := strings.TrimSpace(str(p.VideoURL, "")); url != "" {
			koc.VideoURL = url
			meta, err := s.outreach.FetchVideoMetrics(ctx, url)
			if err != nil {
				return nil, err
			}
			if !meta.PostedAt.IsZero() {
				koc.VideoPostedAt = &meta.PostedAt
			}
		}
		setNum(&koc.VideoViews, p.VideoViews)
		setNum(&koc.VideoOrders, p.VideoOrders)
		setNum(&koc.VideoGmv, p.VideoGmv)
		koc.Stage, koc.KocVideoReply, koc.StalledFlag = domain.StageVideoPosted, "WILL_POST", false
		koc.RaiseCollabStep(4)
	case "remind_video":
		next := koc.VideoReminderStep + 1
		if p.VideoReminderStep != nil && *p.VideoReminderStep != 0 {
			next = *p.VideoReminderStep
		}
		return s.outreach.AdvanceVideoReminderStep(ctx, koc, next)
	case "video_reply":
		reply := orDefault(str(p.KocVideoReply, ""), "WILL_POST")
		koc.KocVideoReply = reply
		switch reply {
		case "WILL_POST":
			koc.StalledFlag = false
		case "NOT_FIT":
			koc.Stage = domain.StageNotFit
			koc.PostReceiveCode = orDefault(str(p.PostReceiveCode, ""), "P01")
			koc.PostReceiveNote = str(p.PostReceiveNote, "")
		case "SILENT":
			koc.Stage, koc.StalledFlag = domain.StageStalled, true
		}
	case "return_product":
		koc.Stage = domain.StageReturning
		if p.PostReceiveCode != nil && *p.PostReceiveCode != "" {
			koc.PostReceiveCode = *p.PostReceiveCode
		}
		setStr(&koc.PostReceiveNote, p.PostReceiveNote)
		setStr(&koc.ReturnTrackingCode, p.ReturnTrackingCode)
		setStr(&koc.ReturnCondition, p.ReturnCondition)
	case "complete_returned":
		koc.Stage, koc.ReturnedAt = domain.StageReturned, &now
		setStr(&koc.ReturnCondition, p.ReturnCondition)
	case "complete_collab":
		setNum(&koc.VideoViews, p.VideoViews)
		setNum(&koc.VideoOrders, p.VideoOrders)
		if p.VideoGmv != nil {
			koc.VideoGmv = *p.VideoGmv
			if koc.VideoGmv > 0 && koc.VideoGmv > koc.Gmv {
				koc.Gmv = koc.VideoGmv
			}
		}
		koc.Stage, koc.CollabStep, koc.StalledFlag = domain.StageCompleted, 5, false
		if p.MarkDaHopTac == nil || *p.MarkDaHopTac {
			koc.DaHopTac = true
		}
	case "set_stage":
		if p.Stage != nil && *p.Stage != "" {
			koc.Stage = *p.Stage
		}
		setStr(&koc.RejectCode, p.RejectCode)
		setStr(&koc.RejectNote, p.RejectNote)
	default:
		return nil, httpx.NewError(400, "Hành động cập nhật không hợp lệ")
	}

	return koc, s.kocs.Save(ctx, koc)
}

// parseClientTime accepts RFC3339 timestamps and plain dates.
func parseClientTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
