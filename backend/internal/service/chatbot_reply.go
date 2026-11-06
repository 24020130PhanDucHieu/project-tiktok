package service

import "strings"

// The chatbot writes like the booker does in the shop's own chats: short, friendly, "bạn / bên mình / nha", no label in
// front, no double quotes, no jargon (system, booker, code). Every reply of the chatbot comes from botText so the
// wording lives in one place and the live chatbot and the preview can never differ.

// replyData is what a reply may mention.
type replyData struct {
	Name    string  // the KOC's display name ("" or "KOC @id" when unknown)
	Rate    float64 // commission rate, percent
	CastFee float64 // approved cast fee, 0 when none
	Phone   string  // phone the KOC just gave, if any
}

// callName is how the chatbot addresses the KOC: the real name when there is one, otherwise just bạn.
func callName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" || strings.HasPrefix(n, "KOC @") {
		return "bạn"
	}
	return n
}

// botText is the reply for an intent, or "" when the chatbot has nothing to say.
func botText(intent string, d replyData) string {
	n := callName(d.Name)
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
		return first + " Hoa hồng affiliate là " + formatRate(rate) + "% trên mỗi đơn, mẫu giao trong khoảng 2-4 ngày, nhận mẫu xong bạn lên video trong 5-7 ngày giúp mình nha. Bạn gửi mình số điện thoại với địa chỉ nhận hàng nha."
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
