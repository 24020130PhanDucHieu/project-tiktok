package service

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// The chatbot writes like the booker does in the shop's own chats: short, friendly, "bạn / bên mình / nha", no label in
// front, no double quotes, no jargon (system, booker, code). It also remembers the conversation: it knows what the KOC
// has already given (address, phone, video), what it said itself a moment ago, and it never asks twice for something it
// already has or repeats the same message. All wording lives in this file.

// replyData is what the first-time wording of a reply may mention.
type replyData struct {
	Name        string  // the KOC's display name ("" or the TikTok id when unknown)
	TiktokID    string  // used to tell a real name from a handle
	Rate        float64 // commission rate, percent
	CastFee     float64 // approved cast fee, 0 when none
	Phone       string  // phone the KOC just gave, if any
	HaveAddress bool    // the shipping address is already saved
}

// callName is how the chatbot addresses the KOC: the real name when there is one, otherwise just bạn. A TikTok handle
// (giadinhtraicay1202, user_47) is not a name.
func callName(name, tiktokID string) string {
	n := strings.TrimSpace(name)
	switch {
	case n == "", strings.HasPrefix(n, "KOC @"), strings.EqualFold(n, strings.TrimPrefix(tiktokID, "@")):
		return "bạn"
	case !strings.ContainsAny(n, " \t") && strings.ContainsAny(n, "0123456789_."):
		return "bạn"
	}
	return n
}

// botText is the first-time reply for an intent, or "" when the chatbot has nothing to say.
func botText(intent string, d replyData) string {
	n := callName(d.Name, d.TiktokID)
	switch intent {
	case "PROVIDED_ADDRESS":
		t := "Okie " + n + " nha, bên mình nhận được thông tin rồi. Mẫu sẽ gửi qua J&T, khoảng 2-4 ngày là tới. Nếu dùng không hợp thì bạn gửi lại giúp bên mình nha."
		if d.Phone != "" {
			t += " Bạn để ý điện thoại số " + d.Phone + " giúp mình, bên giao hàng sẽ gọi nha."
		}
		return t
	case "AGREE_NO_ADDRESS":
		return "Cảm ơn " + n + " nha. Bạn cho mình xin tên người nhận, số điện thoại với địa chỉ nhận hàng để bên mình gửi mẫu qua cho bạn nha."
	case "NEGOTIATE_CAST":
		return "Mình ghi nhận mức bạn đề xuất rồi nha. Mình báo lại với chủ shop rồi phản hồi bạn sớm nhất có thể nha."
	case "ASK_POLICY":
		rate := d.Rate
		if rate == 0 {
			rate = 10
		}
		first := "Bên mình gửi mẫu cho bạn trải nghiệm trước nha, nếu không hợp thì bạn gửi lại giúp bên mình."
		if d.CastFee > 0 {
			first = "Phí cast bên mình duyệt là " + FormatVND(d.CastFee) + "đ nha."
		}
		tail := " Bạn gửi mình số điện thoại với địa chỉ nhận hàng nha."
		if d.HaveAddress {
			tail = " Thông tin nhận hàng của bạn bên mình có rồi nha."
		}
		return first + " Hoa hồng affiliate là " + formatRate(rate) + "% trên mỗi đơn, mẫu giao trong khoảng 2-4 ngày, nhận mẫu xong bạn lên video trong 5-7 ngày giúp mình nha." + tail
	case "CONFIRMED_RECEIVED":
		return "Okie " + n + " nha. Bạn dùng thử rồi lên video theo brief giúp bên mình nha."
	case "SUBMITTED_VIDEO":
		return "Mình nhận được video rồi nha, cảm ơn " + n + " nhiều."
	case "NOT_FIT_RETURN":
		return "Okie " + n + ", không hợp thì không sao nha. Bạn đóng gói gửi lại mẫu về kho giúp bên mình trong 7 ngày, gửi xong nhắn mình mã vận đơn nha."
	case "DECLINED":
		return "Okie " + n + ", cảm ơn bạn đã phản hồi nha. Có đợt hợp tác sau bên mình lại nhắn bạn nha."
	case "WILL_POST":
		return "Okie " + n + " nha. Lên video xong bạn gửi mình link để bên mình ghi nhận nha."
	}
	return ""
}

// dialog is everything the chatbot knows when it answers one message.
type dialog struct {
	Intent      string // what the KOC's message means (ParsedInbound.Intent)
	Text        string // the message itself
	Name        string
	TiktokID    string
	Rate        float64
	CastFee     float64
	Phone       string   // phone in this message
	Stage       string   // the KOC's stage BEFORE this message changed it
	HadAddress  bool     // an address was already saved before this message
	SameAddress bool     // this message repeats the saved address
	SameVideo   bool     // this message repeats the saved video link
	Recent      []string // intents of the bot's latest replies, newest first
	LastBotText string   // the newest message the shop side sent (bot or person), "" when none
}

// said reports whether the bot answered this intent among its few latest replies.
func (d dialog) said(intent string) bool {
	for i, r := range d.Recent {
		if i >= 4 {
			break
		}
		if r == intent {
			return true
		}
	}
	return false
}

var nudgeRe = regexp.MustCompile(`^(alo|a lô|hello|helo|hi|hey|chào|xin chào|ê|shop ơi|bạn ơi|ơi|còn đó không|có ai không|có đây không|alo alo)\b`)

// isNudge is a short greeting or "are you there": nothing to classify, but the KOC is waiting for a sign.
func isNudge(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	return utf8.RuneCountInString(t) <= 30 && nudgeRe.MatchString(t)
}

func deliveredOrLater(stage string) bool {
	switch stage {
	case "DELIVERED", "VIDEO_POSTED", "COMPLETED", "RETURNED":
		return true
	}
	return false
}

// chooseReply decides what the chatbot says, and the intent to remember it by ("" text: stay silent). The same message
// is never sent twice in a row.
func chooseReply(d dialog) (text, intent string) {
	rd := replyData{Name: d.Name, TiktokID: d.TiktokID, Rate: d.Rate, CastFee: d.CastFee, Phone: d.Phone, HaveAddress: d.HadAddress}
	n := callName(d.Name, d.TiktokID)
	intent = d.Intent

	switch d.Intent {
	case "PROVIDED_ADDRESS":
		switch {
		case d.SameAddress:
			text = "Địa chỉ này bên mình có rồi nha. Mẫu sẽ gửi cho bạn sớm nha."
		case d.HadAddress:
			text = "Mình cập nhật địa chỉ mới của bạn rồi nha. Mẫu sẽ gửi theo địa chỉ này nha."
		}
	case "AGREE_NO_ADDRESS":
		switch {
		case d.HadAddress:
			text = "Okie " + n + " nha, thông tin nhận hàng bên mình có rồi. Mẫu sẽ gửi cho bạn sớm nha."
		case d.said("AGREE_NO_ADDRESS"):
			text = "Bên mình vẫn đang chờ tên người nhận, số điện thoại với địa chỉ nhận hàng của bạn nha."
		}
	case "ASK_POLICY":
		if d.said("ASK_POLICY") {
			rate := d.Rate
			if rate == 0 {
				rate = 10
			}
			text = "Hoa hồng vẫn là " + formatRate(rate) + "% trên mỗi đơn nha, phần còn lại như mình nhắn ở trên. Bạn cần hỏi thêm gì cứ nhắn mình nha."
		}
	case "SUBMITTED_VIDEO":
		if d.SameVideo {
			text = "Video này bên mình ghi nhận rồi nha, cảm ơn " + n + " nhiều."
		}
	case "CONFIRMED_RECEIVED":
		if deliveredOrLater(d.Stage) {
			text = "Okie " + n + " nha, có gì cần hỗ trợ bạn cứ nhắn mình nha."
		}
	case "GENERAL_REPLY":
		if isNudge(d.Text) {
			intent = "NUDGE"
			text = nudgeReply(d)
		}
	}
	if text == "" && intent == d.Intent {
		text = botText(d.Intent, rd)
	}
	if text != "" && text == strings.TrimSpace(d.LastBotText) {
		return "", intent // never the same message twice in a row
	}
	return text, intent
}

// nudgeReply answers "alo" / "hello": the KOC is waiting, so say what is still needed from them (once).
func nudgeReply(d dialog) string {
	if len(d.Recent) > 0 && d.Recent[0] == "NUDGE" {
		return "" // already answered, do not chase
	}
	switch {
	case !d.HadAddress && (d.said("AGREE_NO_ADDRESS") || d.said("ASK_POLICY")):
		return "Mình đây nha. Bạn gửi mình tên người nhận, số điện thoại với địa chỉ nhận hàng nha."
	case deliveredOrLater(d.Stage):
		return "Mình đây nha, bạn dùng thử mẫu xong có gì cứ nhắn mình nha."
	}
	return "Mình đây nha, bạn cần mình hỗ trợ gì không nha."
}
