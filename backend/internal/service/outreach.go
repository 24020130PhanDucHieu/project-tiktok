package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

const (
	maxProactivePerDay = 80
	dayDuration        = 24 * time.Hour
)

// vnZone is Vietnam time (UTC+7, no DST); avoids depending on the tz database.
var vnZone = time.FixedZone("Asia/Ho_Chi_Minh", 7*3600)

// OutreachService automates KOC outreach, follow-ups, shipping and chatbot replies.
type OutreachService struct {
	kocs    OutreachKocs
	convs   OutreachConvs
	msgs    OutreachMsgs
	mailer  Mailer
	tracker Tracker
	tiktok  TikTokClient
	zalo    *ZaloService
	gate    AutomationGate // the master switch for messages the system sends by itself; nil means off

	mu       sync.Mutex
	dayKey   string
	dayCount int
}

// NewOutreachService wires the outreach service.
func NewOutreachService(
	kocs OutreachKocs, convs OutreachConvs, msgs OutreachMsgs,
	mailer Mailer, tracker Tracker, tiktok TikTokClient, zalo *ZaloService,
) *OutreachService {
	return &OutreachService{kocs: kocs, convs: convs, msgs: msgs, mailer: mailer, tracker: tracker, tiktok: tiktok, zalo: zalo}
}

// Zalo exposes the Zalo delivery service (used by handlers).
func (s *OutreachService) Zalo() *ZaloService { return s.zalo }

// ---- anti-spam guard ------------------------------------------------------------------------

// vnDayKey names the Vietnam calendar day of t: the quota day starts at 00:00 in Vietnam, not at 07:00 (UTC midnight).
func vnDayKey(t time.Time) string { return t.In(vnZone).Format("2006-01-02") }

// startOfVNDay is 00:00 of t's Vietnam day.
func startOfVNDay(t time.Time) time.Time {
	v := t.In(vnZone)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, vnZone)
}

// resetDailyLocked starts a new count when the Vietnam day changes. The first count of a day (and the first one after
// a restart) is read back from the proactive messages already stored today, so a restart does not give the whole
// quota back.
func (s *OutreachService) resetDailyLocked() {
	now := time.Now()
	if key := vnDayKey(now); s.dayKey != key {
		s.dayKey, s.dayCount = key, s.proactiveSentToday(now)
	}
}

// proactiveSentToday counts today's proactive messages in the store. When the store cannot answer it starts from 0,
// which is the old behaviour, rather than blocking every send.
func (s *OutreachService) proactiveSentToday(now time.Time) int {
	if s.msgs == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n, err := s.msgs.CountProactiveSince(ctx, startOfVNDay(now))
	if err != nil {
		log.Printf("daily quota: cannot read today's count, starting from 0: %v", err)
		return 0
	}
	return int(n)
}

func (s *OutreachService) dailyQuotaReached() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDailyLocked()
	return s.dayCount >= maxProactivePerDay
}

func (s *OutreachService) countProactive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetDailyLocked()
	s.dayCount++
}

// IsWithinAllowedSendWindow allows proactive sends only between 08:30 and 20:30 Vietnam time.
func IsWithinAllowedSendWindow(t time.Time) bool {
	vn := t.In(vnZone)
	minutes := vn.Hour()*60 + vn.Minute()
	return minutes >= 8*60+30 && minutes <= 20*60+30
}

// SendCheck is the verdict of the anti-spam guard.
type SendCheck struct {
	Allowed bool
	Reason  string
}

// CanSendProactiveMessage enforces DNC / collaborated flags, the daily quota, the send window and the 24h rule.
func (s *OutreachService) CanSendProactiveMessage(koc *domain.Koc, bypassWindow bool) SendCheck {
	if koc == nil || koc.Dnc || koc.DaHopTac {
		return SendCheck{Reason: "dnc_or_blocked"}
	}
	if s.dailyQuotaReached() {
		return SendCheck{Reason: "daily_quota_reached"}
	}
	if !bypassWindow && !IsWithinAllowedSendWindow(time.Now()) {
		return SendCheck{Reason: "outside_0830_2030_window"}
	}
	if !bypassWindow && koc.LastContactedAt != nil && time.Since(*koc.LastContactedAt) < dayDuration {
		return SendCheck{Reason: "already_sent_within_24h"}
	}
	return SendCheck{Allowed: true}
}

// ---- message building -----------------------------------------------------------------------

// FormatVND formats a number with Vietnamese thousands separators (500000 -> "500.000").
func FormatVND(n float64) string {
	s := fmt.Sprintf("%.0f", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func commission(koc *domain.Koc) float64 {
	if koc.CommissionRate == 0 {
		return 10
	}
	return koc.CommissionRate
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// BuildInviteMessage composes the first invitation. Policy: never say "tặng" (gift) and never praise "dùng thật".
// It is written as plain sentences, with no tags, quotation marks or explanations in brackets.
func BuildInviteMessage(koc *domain.Koc, customBrief string) string {
	product := orDefault(koc.ProductName, "sản phẩm mẫu của nhãn hàng")
	campaign := "đợt hợp tác sắp tới"
	if c := strings.TrimSpace(koc.Campaign); c != "" {
		campaign = "chiến dịch " + c
	}
	var offer string
	if koc.CastFee > 0 {
		offer = fmt.Sprintf("phí cast đã duyệt %sđ kèm hoa hồng Affiliate %v%%", FormatVND(koc.CastFee), commission(koc))
	} else {
		offer = fmt.Sprintf("gửi sản phẩm mẫu để bạn trải nghiệm, nếu không hợp bạn có thể gửi hoàn lại, kèm hoa hồng Affiliate %v%%", commission(koc))
	}
	brief := "\nBrief: Quay 1 video giới thiệu hoặc dựng bối cảnh phù hợp với phong cách kênh của bạn và gắn giỏ hàng " + product + "."
	if strings.TrimSpace(customBrief) != "" {
		brief = "\nBrief: " + strings.TrimSpace(customBrief)
	}
	variants := []string{
		fmt.Sprintf("Chào %s, bên mình có theo dõi kênh @%s và thấy định hướng nội dung của bạn rất hợp với %s cho dòng %s. Chính sách đợt này là %s.%s\nBạn xem qua và phản hồi giúp bên mình nhé!", koc.DisplayName, koc.TiktokID, campaign, product, offer, brief),
		fmt.Sprintf("Xin chào %s, nhãn hàng bên mình đang triển khai %s với sản phẩm %s. Bên mình đề xuất %s.%s\nNếu phù hợp với lịch của kênh, bạn nhắn lại để bên mình gửi sản phẩm mẫu qua nhé!", koc.DisplayName, campaign, product, offer, brief),
		fmt.Sprintf("Chào bạn %s, mình liên hệ từ đội ngũ Booking của nhãn hàng. Bên mình muốn hợp tác cùng kênh @%s trong %s với sản phẩm %s, chính sách là %s.%s\nBạn phản hồi lại giúp mình để bên mình chuẩn bị mẫu gửi bạn trải nghiệm nhé!", koc.DisplayName, koc.TiktokID, campaign, product, offer, brief),
	}
	idx := (len([]rune(koc.TiktokID)) + time.Now().Day()) % len(variants)
	return variants[idx]
}

// ---- delivery ---------------------------------------------------------------------------------

// Delivery is the result of deliverMessageToKoc.
type Delivery struct {
	Conversation *domain.Conversation
	Message      *domain.Message
	Status       string
}

// DeliverMessageToKoc stores a message in the KOC conversation and pushes it out through Zalo when possible.
func (s *OutreachService) DeliverMessageToKoc(ctx context.Context, koc *domain.Koc, text string, isProactive bool) (*Delivery, error) {
	if koc.Dnc {
		return &Delivery{Status: "blocked_dnc"}, nil
	}
	handle := "@" + koc.TiktokID
	conv, err := s.convs.UpsertKocMessage(ctx, domain.KocContact{Handle: handle, Name: koc.DisplayName, Phone: koc.Phone, LastMessage: text}, time.Now())
	if err != nil {
		return nil, err
	}

	if conv.ExternalContactID == "" && conv.ContactPhone != "" {
		// Keep the internal channel when Zalo is unavailable.
		if user, err := s.zalo.Personal().ResolveByPhone(ctx, conv.ContactPhone); err == nil && user != nil && user.UserID != "" {
			conv.Channel = domain.ChannelZalo
			conv.ExternalContactID = user.UserID
			if err := s.convs.SetZaloContact(ctx, conv.ID, domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: user.UserID}); err != nil {
				return nil, err
			}
		}
	}

	status := domain.MessageSent
	if conv.Channel == domain.ChannelZalo && conv.ExternalContactID != "" {
		// sent only when Zalo really took the message: with the personal line down and no official-account token
		// SendText answers Delivered=false without an error
		if res, err := s.zalo.SendText(ctx, conv.ExternalContactID, text); err != nil || !res.Delivered {
			status = domain.MessagePending
		}
	}
	if isProactive {
		s.countProactive()
	}
	msg := &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Text: text, Status: status, Proactive: isProactive}
	if err := s.msgs.Create(ctx, msg); err != nil {
		return nil, err
	}
	return &Delivery{Conversation: conv, Message: msg, Status: status}, nil
}

// recordInternalNote files a note for the booker in the KOC's thread (a "system" message, like the collaboration
// notes). Unlike DeliverMessageToKoc it never leaves the app: nothing is pushed through Zalo or email.
func (s *OutreachService) recordInternalNote(ctx context.Context, koc *domain.Koc, text string) error {
	conv, err := s.convs.UpsertKocMessage(ctx, domain.KocContact{Handle: "@" + koc.TiktokID, Name: koc.DisplayName, Phone: koc.Phone, LastMessage: text}, time.Now())
	if err != nil {
		return err
	}
	return s.msgs.Create(ctx, &domain.Message{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Kind: "system", Text: text, Status: domain.MessageDelivered})
}

// markInvited moves a KOC to CONTACTED after invitation #1.
func markInvited(koc *domain.Koc, customBrief string) {
	now := time.Now()
	koc.Stage = domain.StageContacted
	koc.InvitedAt = &now
	koc.LastContactedAt = &now
	koc.ReminderStep = 0
	koc.CallTaskPending = false
	koc.DormantUntil = nil
	if customBrief != "" {
		koc.InviteBrief = customBrief
	}
}

// SendFirstInvite sends invitation #1 by itself and moves the KOC to CONTACTED. It is the bulk path, so it needs the
// automation switch; a single booker's click uses PrepareFirstInvite and sends nothing.
func (s *OutreachService) SendFirstInvite(ctx context.Context, koc *domain.Koc, customBrief string, bypassWindow bool) (*domain.Koc, error) {
	if !s.automationOn(ctx) {
		return nil, ErrAutomationOff
	}
	if check := s.CanSendProactiveMessage(koc, bypassWindow); !check.Allowed {
		return nil, fmt.Errorf("Chặn gửi tin chủ động (%s)", check.Reason)
	}
	text := BuildInviteMessage(koc, orDefault(customBrief, koc.InviteBrief))
	if _, err := s.DeliverMessageToKoc(ctx, koc, text, true); err != nil {
		return nil, err
	}
	markInvited(koc, customBrief)
	return koc, s.kocs.Save(ctx, koc)
}

// PrepareFirstInvite records invitation #1 for a booker who is about to send it from the chat, and returns the text
// to put into the chat box. Nothing is sent.
func (s *OutreachService) PrepareFirstInvite(ctx context.Context, koc *domain.Koc, customBrief string) (*domain.Koc, string, error) {
	if koc.Dnc || koc.DaHopTac {
		return nil, "", domain.Conflict("KOC này đã báo không nhận tin hoặc đã hợp tác, không soạn lời mời.")
	}
	text := BuildInviteMessage(koc, orDefault(customBrief, koc.InviteBrief))
	markInvited(koc, customBrief)
	return koc, text, s.kocs.Save(ctx, koc)
}

// ErrOutreachBlocked marks guard rejections so handlers can map them to a client error.
var ErrOutreachBlocked = errors.New("outreach blocked")
