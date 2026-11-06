package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// DigestStats are the counters shown in the morning digest.
type DigestStats struct {
	PendingApproval  int `json:"pendingApproval"`
	NeedCall         int `json:"needCall"`
	NeedShip         int `json:"needShip"`
	DelayedShipments int `json:"delayedShipments"`
	StalledKocs      int `json:"stalledKocs"`
	PendingReturn    int `json:"pendingReturn"`
}

// Digest is the 08:00 summary of today's work.
type Digest struct {
	Text  string      `json:"text"`
	Stats DigestStats `json:"stats"`
}

// BuildMorningDigest summarises today's work: calls, shipping, delayed orders, STALLED and returns.
func (s *OutreachService) BuildMorningDigest(ctx context.Context) (*Digest, error) {
	kocs, err := s.kocs.FindActive(ctx)
	if err != nil {
		return nil, err
	}
	var st DigestStats
	for _, k := range kocs {
		if k.CallTaskPending {
			st.NeedCall++
		}
		if k.Stage == domain.StageAgreed {
			st.NeedShip++
		}
		if k.ShippingDelayedAlert {
			st.DelayedShipments++
		}
		if k.Stage == domain.StageStalled || k.StalledFlag {
			st.StalledKocs++
		}
		if k.Stage == domain.StageAwaitingApproval {
			st.PendingApproval++
		}
		if k.Stage == domain.StageNotFit || k.Stage == domain.StageReturning {
			st.PendingReturn++
		}
	}
	text := strings.Join([]string{
		"[BẢN TÓM TẮT 08:00 SÁNG · VIỆC CẦN LÀM HÔM NAY]",
		fmt.Sprintf("• Cần duyệt phí cast / đàm phán: %d KOC", st.PendingApproval),
		fmt.Sprintf("• Cần gọi điện (quá 7 ngày chưa rep): %d KOC", st.NeedCall),
		fmt.Sprintf("• Chờ gửi mẫu (đã có địa chỉ): %d KOC", st.NeedShip),
		fmt.Sprintf("• Đơn J&T quá 5 ngày chưa giao: %d đơn", st.DelayedShipments),
		fmt.Sprintf("• Cờ đỏ STALLED (quá 3 lần nhắc video): %d KOC", st.StalledKocs),
		fmt.Sprintf("• Đang chờ thu hồi hoàn mẫu (NOT_FIT): %d KOC", st.PendingReturn),
	}, "\n")
	return &Digest{Text: text, Stats: st}, nil
}

// SweepSummary counts what a sweep did.
type SweepSummary struct {
	ReopenedDormant       int `json:"reopenedDormant"`
	ZaloReminded          int `json:"zaloReminded"`
	EmailReminded         int `json:"emailReminded"`
	CallTasksCreated      int `json:"callTasksCreated"`
	SelfShipAsked         int `json:"selfShipAsked"`
	ShipmentsChecked      int `json:"shipmentsChecked"`
	ShipmentsDelivered    int `json:"shipmentsDelivered"`
	ShippingDelayedAlerts int `json:"shippingDelayedAlerts"`
	TiktokDailyScanned    int `json:"tiktokDailyScanned"`
	VideoReminded         int `json:"videoReminded"`
	StalledFlagged        int `json:"stalledFlagged"`
	ReturnReminded        int `json:"returnReminded"`
}

func daysSince(t *time.Time, fallback time.Time, now time.Time) float64 {
	if t == nil {
		return now.Sub(fallback).Hours() / 24
	}
	return now.Sub(*t).Hours() / 24
}

// RunStage2Sweep is the periodic automation (every 4h): reopen dormant KOCs, chase silent KOCs, check
// shipments, scan TikTok for the collab video, remind about videos and returns.
// With simulateNextStep it advances each KOC by exactly one step (demo mode).
func (s *OutreachService) RunStage2Sweep(ctx context.Context, simulateNextStep bool) (*SweepSummary, error) {
	now := time.Now()
	sum := &SweepSummary{}

	// Reopen DORMANT KOCs after 30 days.
	dormant, err := s.kocs.FindDormantDue(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, k := range dormant {
		k.Stage, k.ReminderStep, k.CallTaskPending, k.DormantUntil = domain.StageQualified, 0, false, nil
		if err := s.kocs.Save(ctx, k); err != nil {
			return nil, err
		}
		sum.ReopenedDormant++
	}

	// Chase KOCs in CONTACTED / NO_REPLY: +2d email, +4d Zalo, +7d call task.
	outreach, err := s.kocs.FindToChase(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range outreach {
		hours := 0.0
		if k.InvitedAt != nil {
			hours = now.Sub(*k.InvitedAt).Hours()
		}
		if simulateNextStep {
			switch k.ReminderStep {
			case 0:
				if _, err := s.AdvanceFollowUpStep(ctx, k, 1); err != nil {
					return nil, err
				}
				sum.EmailReminded++
			case 1:
				if _, err := s.AdvanceFollowUpStep(ctx, k, 2); err != nil {
					return nil, err
				}
				sum.ZaloReminded++
			case 2:
				if _, err := s.AdvanceFollowUpStep(ctx, k, 3); err != nil {
					return nil, err
				}
				sum.CallTasksCreated++
			}
			continue
		}
		if !s.CanSendProactiveMessage(k, false).Allowed {
			continue
		}
		switch {
		case hours >= 24*7 && k.ReminderStep < 3:
			if _, err := s.AdvanceFollowUpStep(ctx, k, 3); err != nil {
				return nil, err
			}
			sum.CallTasksCreated++
		case hours >= 24*4 && k.ReminderStep < 2:
			if _, err := s.AdvanceFollowUpStep(ctx, k, 2); err != nil {
				return nil, err
			}
			sum.ZaloReminded++
		case hours >= 48 && k.ReminderStep < 1:
			if _, err := s.AdvanceFollowUpStep(ctx, k, 1); err != nil {
				return nil, err
			}
			sum.EmailReminded++
		}
	}

	// Shipping: self-ship follow-up after 3 days, J&T status every sweep, delay alert after 5 days.
	shipping, err := s.kocs.FindInShipping(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range shipping {
		days := daysSince(k.ShippedAt, now, now)
		if k.ShippingCarrier == "Tự vận chuyển" || k.TrackingCode == "" {
			if (days >= 3 || simulateNextStep) && k.SelfShipAskedAt == nil {
				msg := fmt.Sprintf("Chào %s, sản phẩm mẫu bên mình tự vận chuyển gửi đi được 3 ngày rồi ạ. Bạn đã nhận được hàng chưa? Nếu nhận được rồi bạn nhắn lại chữ rồi giúp mình nhé!", k.DisplayName)
				if _, err := s.DeliverMessageToKoc(ctx, k, msg, true); err != nil {
					return nil, err
				}
				k.SelfShipAskedAt = &now
				if err := s.kocs.Save(ctx, k); err != nil {
					return nil, err
				}
				sum.SelfShipAsked++
			}
			if days > 5 {
				k.ShippingDelayedAlert = true
				if err := s.kocs.Save(ctx, k); err != nil {
					return nil, err
				}
				sum.ShippingDelayedAlerts++
			}
			continue
		}
		sum.ShipmentsChecked++
		res, err := s.SyncKocShipment(ctx, k)
		if err != nil {
			return nil, err
		}
		if res.Tracking != nil && res.Tracking.Delivered {
			sum.ShipmentsDelivered++
		}
		if k.ShippingDelayedAlert {
			sum.ShippingDelayedAlerts++
		}
	}

	// Delivered: scan TikTok for the collab video, otherwise remind at +5/+8/+12 days, then STALLED.
	delivered, err := s.kocs.FindDelivered(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range delivered {
		if _, err := s.SyncKocTikTokChannel(ctx, k, true); err == nil {
			sum.TiktokDailyScanned++
		}
		if k.Stage == domain.StageVideoPosted {
			continue
		}
		days := daysSince(k.DeliveredAt, now, now)

		if simulateNextStep {
			next := k.VideoReminderStep + 1
			if _, err := s.AdvanceVideoReminderStep(ctx, k, next); err != nil {
				return nil, err
			}
			if next >= 4 {
				sum.StalledFlagged++
			} else {
				sum.VideoReminded++
			}
			continue
		}
		switch {
		case days > 12 && k.VideoReminderStep >= 3 && k.VideoURL == "":
			if _, err := s.AdvanceVideoReminderStep(ctx, k, 4); err != nil {
				return nil, err
			}
			sum.StalledFlagged++
		case days >= 12 && k.VideoReminderStep < 3:
			if _, err := s.AdvanceVideoReminderStep(ctx, k, 3); err != nil {
				return nil, err
			}
			sum.VideoReminded++
		case days >= 8 && k.VideoReminderStep < 2:
			if _, err := s.AdvanceVideoReminderStep(ctx, k, 2); err != nil {
				return nil, err
			}
			sum.VideoReminded++
		case days >= 5 && k.VideoReminderStep < 1:
			if _, err := s.AdvanceVideoReminderStep(ctx, k, 1); err != nil {
				return nil, err
			}
			sum.VideoReminded++
		}
	}

	// NOT_FIT: remind to send the sample back after 7 days.
	notFit, err := s.kocs.FindNotFitAwaitingReturn(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range notFit {
		days := now.Sub(k.UpdatedAt).Hours() / 24
		if (days >= 7 || simulateNextStep) && k.ReturnReminderSentAt == nil {
			msg := fmt.Sprintf("Chào %s, sản phẩm mẫu không phù hợp bạn đã gửi lại giúp shop chưa ạ? Bạn cho mình xin mã vận đơn gửi hoàn nhé!", k.DisplayName)
			if _, err := s.DeliverMessageToKoc(ctx, k, msg, true); err != nil {
				return nil, err
			}
			k.ReturnReminderSentAt = &now
			if err := s.kocs.Save(ctx, k); err != nil {
				return nil, err
			}
			sum.ReturnReminded++
		}
	}
	return sum, nil
}
