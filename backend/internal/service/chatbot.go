package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ParsedInbound is the classification of an inbound KOC message.
type ParsedInbound struct {
	Intent               string `json:"intent"`
	Phone                string `json:"phone"`
	ShippingAddress      string `json:"shippingAddress"`
	SuggestedDeclineCode string `json:"suggestedDeclineCode"`
	IsDecline            bool   `json:"isDecline"`
	IsNegotiateCast      bool   `json:"isNegotiateCast"`
	IsNotFitAfterReceive bool   `json:"isNotFitAfterReceive"`
	IsConfirmedReceived  bool   `json:"isConfirmedReceived"`
	IsWillPost           bool   `json:"isWillPost"`
	IsAgreeIntent        bool   `json:"isAgreeIntent"`
	IsAskPolicy          bool   `json:"isAskPolicy"`
	VideoURL             string `json:"videoUrl"`
}

var (
	addressKeywordRe = regexp.MustCompile(`(?i)(địa chỉ|đ/c|dc:|số nhà|ngõ|hẻm|phường|quận|huyện|tỉnh|thành phố|tp\.|tx\.|thị xã|xã|ấp|thôn|tòa|chung cư|ký túc xá|ktx|đường|phố|cầu giấy|đống đa|thanh xuân|hà nội|hcm|sài gòn|đà nẵng)`)
	dncRe            = regexp.MustCompile(`(đừng nhắn nữa|ngừng làm phiền|stop|hủy nhận tin|dnc)`)
	negotiateRe      = regexp.MustCompile(`(tăng cast|thêm phí cast|tăng hoa hồng|giá cast của mình là|bên mình nhận cast|có phí cast mới nhận|hoa hồng thấp quá)`)
	declineRe        = regexp.MustCompile(`(không nhận|từ chối|chưa phù hợp|không phù hợp|bận lịch|kín lịch|không hợp tác|không làm được|thôi shop|mình xin phép không)`)
	notFitRe         = regexp.MustCompile(`(không hợp sản phẩm|bị kích ứng|không lên video được|xin gửi trả|hoàn hàng|trả lại mẫu)`)
	receivedRe       = regexp.MustCompile(`(nhận được rồi|đã nhận hàng|mình nhận rồi|nhận mẫu rồi|rồi shop ơi)`)
	willPostRe       = regexp.MustCompile(`(sẽ đăng|đang quay|sắp lên video|mai mình lên|tuần này lên|đang dựng video)`)
	agreeRe          = regexp.MustCompile(`(đồng ý|ok shop|okie|nhận nhé|mình nhận|gửi mẫu giúp|hợp tác nhé|chốt nhé|ok bạn)`)
	askPolicyRe      = regexp.MustCompile(`(hoa hồng|phí cast|chính sách|quyền lợi|brief như nào|thời gian nhận hàng|quy trình)`)
	tiktokURLRe      = regexp.MustCompile(`(?i)https?://(?:www\.|vt\.|vm\.)?tiktok\.com/\S+`)
	freecastRe       = regexp.MustCompile(`freecast|có phí`)
	priceRe          = regexp.MustCompile(`giá|ngân sách|cast`)
	commissionRe     = regexp.MustCompile(`hoa hồng`)
	busyRe           = regexp.MustCompile(`bận|kín lịch`)
	notSuitRe        = regexp.MustCompile(`không hợp|định hướng`)
)

// ParseKocInboundText classifies the intent of a KOC message (agree, decline, ask policy, send address/video...).
func ParseKocInboundText(text string) ParsedInbound {
	raw := strings.TrimSpace(text)
	lower := strings.ToLower(raw)

	phone := ""
	if m := phoneRe.FindString(raw); m != "" {
		phone, _ = ExtractContactFromBio(m)
	}
	shipping := ""
	if addressKeywordRe.MatchString(raw) || (phone != "" && len([]rune(raw)) > 22) {
		shipping = raw
	}

	p := ParsedInbound{Phone: phone, ShippingAddress: shipping}
	isDNC := dncRe.MatchString(lower)
	p.IsNegotiateCast = negotiateRe.MatchString(lower)
	p.IsDecline = declineRe.MatchString(lower)
	p.IsNotFitAfterReceive = notFitRe.MatchString(lower)
	p.IsConfirmedReceived = receivedRe.MatchString(lower)
	p.IsWillPost = willPostRe.MatchString(lower)
	p.IsAgreeIntent = agreeRe.MatchString(lower)
	p.IsAskPolicy = askPolicyRe.MatchString(lower)
	p.VideoURL = tiktokURLRe.FindString(raw)

	if p.IsDecline || p.IsNegotiateCast {
		switch {
		case freecastRe.MatchString(lower):
			p.SuggestedDeclineCode = "R01"
		case priceRe.MatchString(lower):
			p.SuggestedDeclineCode = "R02"
		case commissionRe.MatchString(lower):
			p.SuggestedDeclineCode = "R03"
		case busyRe.MatchString(lower):
			p.SuggestedDeclineCode = "R04"
		case notSuitRe.MatchString(lower):
			p.SuggestedDeclineCode = "R05"
		default:
			p.SuggestedDeclineCode = "R09"
		}
	}

	switch {
	case isDNC:
		p.Intent = "SET_DNC"
	case p.VideoURL != "":
		p.Intent = "SUBMITTED_VIDEO"
	case shipping != "":
		p.Intent = "PROVIDED_ADDRESS"
	case p.IsConfirmedReceived:
		p.Intent = "CONFIRMED_RECEIVED"
	case p.IsNotFitAfterReceive:
		p.Intent = "NOT_FIT_RETURN"
	case p.IsNegotiateCast:
		p.Intent = "NEGOTIATE_CAST"
	case p.IsDecline:
		p.Intent = "DECLINED"
	case p.IsAgreeIntent:
		p.Intent = "AGREE_NO_ADDRESS"
	case p.IsAskPolicy:
		p.Intent = "ASK_POLICY"
	case p.IsWillPost:
		p.Intent = "WILL_POST"
	default:
		p.Intent = "GENERAL_REPLY"
	}
	return p
}

// ChatbotKocInfo is the minimal KOC context used for a stateless chatbot preview.
type ChatbotKocInfo struct {
	DisplayName    string
	ProductName    string
	CommissionRate float64
	Dnc            bool
}

// BuildChatbotAutoReply previews the reply for a parsed message (used by /inbox/chatbot/parse).
// It returns "" when the chatbot must stay silent (DNC) or has nothing to say.
func BuildChatbotAutoReply(k ChatbotKocInfo, parsed ParsedInbound) string {
	if k.Dnc {
		return ""
	}
	name := k.DisplayName
	rate := k.CommissionRate
	if rate == 0 {
		rate = 10
	}
	switch parsed.Intent {
	case "PROVIDED_ADDRESS":
		return "[Chatbot tự động] Cảm ơn " + name + "! Hệ thống đã lưu thông tin nhận mẫu trải nghiệm. Thời gian giao hàng dự kiến 2–4 ngày qua J&T Express. Nếu trải nghiệm không hợp bạn có thể gửi hoàn lại giúp shop nhé!"
	case "AGREE_NO_ADDRESS":
		return "[Chatbot tự động] Cảm ơn " + name + " đã đồng ý hợp tác! Bạn cho mình xin Tên người nhận + Số điện thoại + Địa chỉ nhận hàng để bên mình gửi sản phẩm mẫu qua bạn trải nghiệm nhé!"
	case "NEGOTIATE_CAST":
		return "[Chatbot tự động] Mình đã ghi nhận đề xuất về mức phí cast / hoa hồng của " + name + " và chuyển ngay cho Booker & Chủ shop phê duyệt. Bên mình sẽ phản hồi lại bạn sớm nhất nhé!"
	case "ASK_POLICY":
		return "[Chatbot tự động] Chào " + name + ", quy trình bên mình gồm: gửi sản phẩm mẫu để trải nghiệm (nếu không hợp bạn gửi hoàn lại giúp shop), hoa hồng Affiliate " + formatRate(rate) + "% trên đơn hàng, thời gian giao mẫu từ 2–4 ngày và lên video trong 5–7 ngày sau khi nhận mẫu. Bạn gửi giúp mình SĐT + Địa chỉ nhận hàng nhé!"
	case "CONFIRMED_RECEIVED":
		return "[Chatbot tự động] Cảm ơn " + name + " đã xác nhận nhận hàng! Bạn trải nghiệm sản phẩm và chuẩn bị lên video theo brief giúp bên mình nhé."
	case "SUBMITTED_VIDEO":
		return "[Chatbot tự động] Hệ thống đã ghi nhận video TikTok của " + name + " (" + parsed.VideoURL + ") và dừng chuỗi nhắc lịch. Cảm ơn bạn đã đồng hành cùng nhãn hàng!"
	case "NOT_FIT_RETURN":
		return "[Chatbot tự động] Bên mình đã ghi nhận sản phẩm chưa phù hợp với bạn (Mã P01). Theo chính sách gửi mẫu trải nghiệm, bạn vui lòng đóng gói và gửi hoàn lại sản phẩm về kho giúp shop trong vòng 7 ngày và nhắn lại mã vận đơn hoàn vào đây nhé!"
	case "DECLINED":
		code := parsed.SuggestedDeclineCode
		if code == "" {
			code = "R04"
		}
		return "[Chatbot tự động] Cảm ơn " + name + " đã phản hồi. Mình đã ghi nhận thông tin (Đề xuất mã: " + code + ") để Booker cập nhật hồ sơ. Hẹn hợp tác cùng bạn ở đợt tiếp theo nhé!"
	case "WILL_POST":
		return "[Chatbot tự động] Cảm ơn " + name + "! Khi nào đăng video xong bạn gửi link vào đây để hệ thống tự động ghi nhận nhé."
	}
	return ""
}

func formatRate(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// InboundResult is the outcome of processing one inbound KOC message.
type InboundResult struct {
	Parsed     ParsedInbound
	Koc        *domain.Koc
	BotMessage *domain.Message
}

// ProcessInboundWithChatbot applies the chatbot rules: skip DNC KOCs, never negotiate cast/commission on its own,
// never say "tặng", and suggest a decline code for the booker to confirm.
func (s *OutreachService) ProcessInboundWithChatbot(ctx context.Context, conv *domain.Conversation, text string, autoReply bool) (*InboundResult, error) {
	parsed := ParseKocInboundText(text)
	var or []bson.M
	if conv != nil && strings.HasPrefix(conv.ContactHandle, "@") {
		or = append(or, bson.M{"tiktokId": strings.ToLower(conv.ContactHandle[1:])})
	}
	if conv != nil && conv.ContactPhone != "" {
		or = append(or, bson.M{"phone": conv.ContactPhone})
	}
	if parsed.Phone != "" {
		or = append(or, bson.M{"phone": parsed.Phone})
	}

	var koc *domain.Koc
	if len(or) > 0 {
		var err error
		koc, err = s.kocs.FindOne(ctx, bson.M{"status": domain.KocActive, "$or": or})
		if err != nil {
			return nil, err
		}
	}
	res := &InboundResult{Parsed: parsed, Koc: koc}

	// Rule #4: stay silent when the KOC asked not to be contacted.
	if koc != nil && koc.Dnc {
		return res, nil
	}

	reply := ""
	if koc != nil {
		now := time.Now()
		koc.LastRepliedAt = &now
		koc.CallTaskPending = false
		koc.TungKhongRep = false
		if parsed.Phone != "" && koc.Phone == "" {
			koc.Phone = parsed.Phone
		}
		name := koc.DisplayName

		switch parsed.Intent {
		case "SET_DNC":
			koc.Dnc = true
			return res, s.kocs.Save(ctx, koc)
		case "PROVIDED_ADDRESS":
			koc.ShippingAddress = parsed.ShippingAddress
			if koc.Recipient == "" {
				koc.Recipient = koc.DisplayName
			}
			koc.Stage = domain.StageAgreed
			koc.RaiseCollabStep(2)
			phonePart := ""
			if parsed.Phone != "" {
				phonePart = " · SĐT " + parsed.Phone
			}
			reply = "[Chatbot tự động] Cảm ơn " + name + "! Hệ thống đã lưu thông tin nhận mẫu trải nghiệm (\"" + parsed.ShippingAddress + "\")" + phonePart + ". Thời gian giao hàng dự kiến 2–4 ngày qua J&T Express. Nếu trải nghiệm không hợp bạn có thể gửi hoàn lại giúp shop nhé!"
		case "AGREE_NO_ADDRESS":
			koc.Stage = domain.StageReplied
			koc.RaiseCollabStep(1)
			reply = "[Chatbot tự động] Cảm ơn " + name + " đã đồng ý hợp tác! Bạn cho mình xin Tên người nhận + Số điện thoại + Địa chỉ nhận hàng để bên mình gửi sản phẩm mẫu qua bạn trải nghiệm nhé!"
		case "NEGOTIATE_CAST":
			koc.Stage = domain.StageAwaitingApproval
			koc.SuggestedDeclineCode = orDefault(parsed.SuggestedDeclineCode, "R02")
			reply = "[Chatbot tự động] Mình đã ghi nhận đề xuất về mức phí cast / hoa hồng của " + name + " và chuyển ngay cho Booker & Chủ shop phê duyệt. Bên mình sẽ phản hồi lại bạn sớm nhất nhé!"
		case "ASK_POLICY":
			koc.Stage = domain.StageReplied
			koc.RaiseCollabStep(1)
			feeNote := "gửi sản phẩm mẫu để trải nghiệm (nếu không hợp bạn gửi hoàn lại giúp shop)"
			if koc.CastFee > 0 {
				feeNote = "phí cast đã duyệt " + FormatVND(koc.CastFee) + "đ"
			}
			reply = "[Chatbot tự động] Chào " + name + ", quy trình bên mình gồm: " + feeNote + ", hoa hồng Affiliate " + formatRate(commission(koc)) + "% trên đơn hàng, thời gian giao mẫu từ 2–4 ngày và lên video trong 5–7 ngày sau khi nhận mẫu. Bạn gửi giúp mình SĐT + Địa chỉ nhận hàng nhé!"
		case "CONFIRMED_RECEIVED":
			koc.Stage = domain.StageDelivered
			koc.DeliveredAt = &now
			koc.TrackingStatusText = "KOC đã xác nhận nhận được hàng"
			koc.RaiseCollabStep(3)
			reply = "[Chatbot tự động] Cảm ơn " + name + " đã xác nhận nhận hàng! Bạn trải nghiệm sản phẩm và chuẩn bị lên video theo brief giúp bên mình nhé."
		case "SUBMITTED_VIDEO":
			koc.VideoURL = parsed.VideoURL
			koc.VideoPostedAt = &now
			koc.Stage = domain.StageVideoPosted
			koc.RaiseCollabStep(4)
			koc.StalledFlag = false
			reply = "[Chatbot tự động] Hệ thống đã ghi nhận video TikTok của " + name + " (" + parsed.VideoURL + ") và dừng chuỗi nhắc lịch. Cảm ơn bạn đã đồng hành cùng nhãn hàng!"
		case "NOT_FIT_RETURN":
			koc.Stage = domain.StageNotFit
			koc.KocVideoReply = "NOT_FIT"
			koc.PostReceiveCode = orDefault(koc.PostReceiveCode, "P01")
			reply = "[Chatbot tự động] Bên mình đã ghi nhận sản phẩm chưa phù hợp với bạn (Mã P01). Theo chính sách gửi mẫu trải nghiệm, bạn vui lòng đóng gói và gửi hoàn lại sản phẩm về kho giúp shop trong vòng 7 ngày và nhắn lại mã vận đơn hoàn vào đây nhé!"
		case "DECLINED":
			koc.Stage = domain.StageReplied
			koc.SuggestedDeclineCode = orDefault(parsed.SuggestedDeclineCode, "R04")
			reply = "[Chatbot tự động] Cảm ơn " + name + " đã phản hồi. Mình đã ghi nhận thông tin (Đề xuất mã: " + koc.SuggestedDeclineCode + ") để Booker cập nhật hồ sơ. Hẹn hợp tác cùng bạn ở đợt tiếp theo nhé!"
		case "WILL_POST":
			koc.KocVideoReply = "WILL_POST"
			reply = "[Chatbot tự động] Cảm ơn " + name + "! Khi nào đăng video xong bạn gửi link vào đây để hệ thống tự động ghi nhận nhé."
		default:
			switch koc.Stage {
			case domain.StageContacted, domain.StageNoReply, domain.StageDormant:
				koc.Stage = domain.StageReplied
				koc.RaiseCollabStep(1)
			}
		}
		if err := s.kocs.Save(ctx, koc); err != nil {
			return nil, err
		}
	}

	if autoReply && reply != "" && conv != nil {
		bot := &domain.Message{
			ConversationID: conv.ID, Direction: domain.DirectionOutbound,
			Text: reply, Status: "Chatbot đã trả lời",
		}
		if err := s.msgs.Create(ctx, bot); err != nil {
			return nil, err
		}
		conv.LastMessage = reply
		conv.LastMessageAt = bot.SentAt
		if err := s.convs.Save(ctx, conv); err != nil {
			return nil, err
		}
		res.BotMessage = bot
	}
	return res, nil
}
