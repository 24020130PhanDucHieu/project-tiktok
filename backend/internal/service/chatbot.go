package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	Raw                  string `json:"-"` // the message as the KOC wrote it
}

var (
	// strongAddressRe holds phrases people only write when they give an address.
	strongAddressRe = regexp.MustCompile(`(địa chỉ|đ/c|đc:|dc:|số nhà|chung cư|ký túc xá)`)
	dncRe           = regexp.MustCompile(`(đừng nhắn nữa|ngừng làm phiền|stop|hủy nhận tin|dnc)`)
	negotiateRe     = regexp.MustCompile(`(tăng cast|thêm phí cast|tăng hoa hồng|giá cast của mình là|bên mình nhận cast|có phí cast mới nhận|hoa hồng thấp quá)`)
	declineRe       = regexp.MustCompile(`(không nhận|từ chối|chưa phù hợp|không phù hợp|bận lịch|kín lịch|không hợp tác|không làm được|thôi shop|mình xin phép không)`)
	notFitRe        = regexp.MustCompile(`(không hợp sản phẩm|bị kích ứng|không lên video được|xin gửi trả|hoàn hàng|trả lại mẫu)`)
	receivedRe      = regexp.MustCompile(`(nhận được rồi|đã nhận hàng|mình nhận rồi|nhận mẫu rồi|rồi shop ơi)`)
	willPostRe      = regexp.MustCompile(`(sẽ đăng|đang quay|sắp lên video|mai mình lên|tuần này lên|đang dựng video)`)
	agreeRe         = regexp.MustCompile(`(đồng ý|ok shop|okie|nhận nhé|mình nhận|gửi mẫu giúp|hợp tác nhé|chốt nhé|ok bạn)`)
	askPolicyRe     = regexp.MustCompile(`(hoa hồng|phí cast|chính sách|quyền lợi|brief như nào|thời gian nhận hàng|quy trình)`)
	tiktokURLRe     = regexp.MustCompile(`(?i)https?://(?:www\.|vt\.|vm\.)?tiktok\.com/\S+`)
	freecastRe      = regexp.MustCompile(`freecast|có phí`)
	priceRe         = regexp.MustCompile(`giá|ngân sách|cast`)
	commissionRe    = regexp.MustCompile(`hoa hồng`)
	busyRe          = regexp.MustCompile(`bận|kín lịch`)
	notSuitRe       = regexp.MustCompile(`không hợp|định hướng`)
)

// addressPlaceWords are the words of a postal address. Each one is also an everyday word ("xã hội", "đường link",
// "tỉnh táo"), so one of them alone is not proof, and they only count as whole words: "ấp" sits inside "thấp".
var addressPlaceWords = []string{
	"ngõ", "ngách", "hẻm", "phường", "quận", "huyện", "tỉnh", "thành phố", "tp", "tx", "thị xã", "xã", "ấp", "thôn",
	"tòa", "đường", "phố", "ktx", "cầu giấy", "đống đa", "thanh xuân", "hà nội", "hcm", "sài gòn", "đà nẵng",
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// hasWord reports whether word appears in s as a whole word (not inside a longer one). s must be lower case.
func hasWord(s, word string) bool {
	for from := 0; from < len(s); {
		i := strings.Index(s[from:], word)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(word)
		before, after := true, true
		if start > 0 {
			r, _ := utf8.DecodeLastRuneInString(s[:start])
			before = !isWordRune(r)
		}
		if end < len(s) {
			r, _ := utf8.DecodeRuneInString(s[end:])
			after = !isWordRune(r)
		}
		if before && after {
			return true
		}
		from = start + 1
	}
	return false
}

// looksLikeAddress tells a delivery address from a message that merely contains one of its words. It needs a phrase
// only an address has ("địa chỉ", "số nhà"), or two different place words, or one place word with a number.
func looksLikeAddress(raw string) bool {
	lower := strings.ToLower(raw)
	if strongAddressRe.MatchString(lower) {
		return true
	}
	places := 0
	for _, w := range addressPlaceWords {
		if hasWord(lower, w) {
			places++
		}
	}
	if places >= 2 {
		return true
	}
	return places == 1 && strings.IndexFunc(lower, unicode.IsDigit) >= 0
}

// ParseKocInboundText classifies the intent of a KOC message (agree, decline, ask policy, send address/video...).
func ParseKocInboundText(text string) ParsedInbound {
	raw := strings.TrimSpace(text)
	lower := strings.ToLower(raw)

	phone := ""
	if m := phoneRe.FindString(raw); m != "" {
		phone, _ = ExtractContactFromBio(m)
	}
	shipping := ""
	if looksLikeAddress(raw) || (phone != "" && len([]rune(raw)) > 22) {
		shipping = raw
	}

	p := ParsedInbound{Phone: phone, ShippingAddress: shipping, Raw: raw}
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
	text, _ := chooseReply(dialog{Intent: parsed.Intent, Text: parsed.Raw, Name: k.DisplayName, Rate: k.CommissionRate, Phone: parsed.Phone})
	return text
}

// earlyStage is a stage from which a KOC's reply moves it to REPLIED; later stages are left as they are.
func earlyStage(stage string) bool {
	switch stage {
	case domain.StageSourced, domain.StageQualified, domain.StageContacted, domain.StageNoReply, domain.StageDormant, domain.StageReplied, "":
		return true
	}
	return false
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
//
// When the KOC was saved by somebody else between this read and this write (the sweep, a person), the save is refused
// with a Conflict. Nobody can be asked to try again for a KOC's message, so it reads the KOC again and applies the
// message once more. The save is the first thing that can fail here, before any message is stored, so repeating the
// whole step has no side effect to undo.
func (s *OutreachService) ProcessInboundWithChatbot(ctx context.Context, conv *domain.Conversation, text string, autoReply bool) (*InboundResult, error) {
	res, err := s.processInbound(ctx, conv, text, autoReply)
	var fault *domain.Fault
	if errors.As(err, &fault) && fault.Kind == domain.FaultConflict {
		res, err = s.processInbound(ctx, conv, text, autoReply)
	}
	return res, err
}

func (s *OutreachService) processInbound(ctx context.Context, conv *domain.Conversation, text string, autoReply bool) (*InboundResult, error) {
	parsed := ParseKocInboundText(text)
	var handle, convPhone string
	if conv != nil {
		if strings.HasPrefix(conv.ContactHandle, "@") {
			handle = strings.ToLower(conv.ContactHandle[1:])
		}
		convPhone = conv.ContactPhone
	}
	koc, err := s.kocs.FindActiveByContact(ctx, handle, convPhone, parsed.Phone)
	if err != nil {
		return nil, err
	}
	res := &InboundResult{Parsed: parsed, Koc: koc}

	// Rule #4: stay silent when the KOC asked not to be contacted.
	if koc != nil && koc.Dnc {
		return res, nil
	}

	reply, replyIntent := "", ""
	if koc != nil {
		// What the chat already contains: what the KOC gave before this message, and what the bot said lately.
		dlg := dialog{
			Intent: parsed.Intent, Text: parsed.Raw, Name: koc.DisplayName, TiktokID: koc.TiktokID, Rate: commission(koc), CastFee: koc.CastFee,
			Phone: parsed.Phone, Stage: koc.Stage,
			HadAddress:  strings.TrimSpace(koc.ShippingAddress) != "",
			SameAddress: parsed.ShippingAddress != "" && strings.TrimSpace(koc.ShippingAddress) == strings.TrimSpace(parsed.ShippingAddress),
			SameVideo:   parsed.VideoURL != "" && koc.VideoURL == parsed.VideoURL,
		}
		if conv != nil {
			if recent, err := s.msgs.Recent(ctx, conv.ID, 12); err == nil {
				for _, m := range recent {
					if m.Direction != domain.DirectionOutbound {
						continue
					}
					if dlg.LastBotText == "" {
						dlg.LastBotText = m.Text
					}
					if m.Intent != "" {
						dlg.Recent = append(dlg.Recent, m.Intent)
					}
				}
			}
		}
		now := time.Now()
		koc.LastRepliedAt = &now
		koc.CallTaskPending = false
		koc.TungKhongRep = false
		if parsed.Phone != "" && koc.Phone == "" {
			koc.Phone = parsed.Phone
		}

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
		case "AGREE_NO_ADDRESS":
			if earlyStage(koc.Stage) { // never move a KOC backwards because it agreed again
				koc.Stage = domain.StageReplied
				koc.RaiseCollabStep(1)
			}
		case "NEGOTIATE_CAST":
			koc.Stage = domain.StageAwaitingApproval
			koc.SuggestedDeclineCode = orDefault(parsed.SuggestedDeclineCode, "R02")
		case "ASK_POLICY":
			if earlyStage(koc.Stage) {
				koc.Stage = domain.StageReplied
				koc.RaiseCollabStep(1)
			}
		case "CONFIRMED_RECEIVED":
			koc.Stage = domain.StageDelivered
			koc.DeliveredAt = &now
			koc.TrackingStatusText = "KOC đã xác nhận nhận được hàng"
			koc.RaiseCollabStep(3)
		case "SUBMITTED_VIDEO":
			koc.VideoURL = parsed.VideoURL
			koc.VideoPostedAt = &now
			koc.Stage = domain.StageVideoPosted
			koc.RaiseCollabStep(4)
			koc.StalledFlag = false
		case "NOT_FIT_RETURN":
			koc.Stage = domain.StageNotFit
			koc.KocVideoReply = "NOT_FIT"
			koc.PostReceiveCode = orDefault(koc.PostReceiveCode, "P01")
		case "DECLINED":
			koc.Stage = domain.StageReplied
			koc.SuggestedDeclineCode = orDefault(parsed.SuggestedDeclineCode, "R04")
		case "WILL_POST":
			koc.KocVideoReply = "WILL_POST"
		default:
			switch koc.Stage {
			case domain.StageContacted, domain.StageNoReply, domain.StageDormant:
				koc.Stage = domain.StageReplied
				koc.RaiseCollabStep(1)
			}
		}
		reply, replyIntent = chooseReply(dlg)
		if err := s.kocs.Save(ctx, koc); err != nil {
			return nil, err
		}
	}

	if autoReply && reply != "" && conv != nil {
		bot := &domain.Message{
			ConversationID: conv.ID, Direction: domain.DirectionOutbound,
			Text: reply, Intent: replyIntent, Status: "Chatbot đã trả lời",
		}
		if err := s.msgs.Create(ctx, bot); err != nil {
			return nil, err
		}
		conv.LastMessage = reply
		conv.LastMessageAt = bot.SentAt
		if err := s.convs.SetLastMessageAt(ctx, conv.ID, reply, bot.SentAt); err != nil {
			return nil, err
		}
		res.BotMessage = bot
	}
	return res, nil
}
