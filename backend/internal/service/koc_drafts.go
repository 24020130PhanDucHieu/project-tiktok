package service

import (
	"fmt"
	"strings"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// The messages below are written for a person to read on Zalo. They are plain sentences: no [tags], no quotation
// marks and no explanations in brackets. The stage buttons hand them back as drafts for the booker to read, change
// and send; only the background sweep (with the automation switch on) sends them by itself.

// kocName is how the message addresses the creator.
func kocName(k *domain.Koc) string {
	if n := strings.TrimSpace(k.DisplayName); n != "" {
		return n
	}
	return "bạn"
}

func sampleName(k *domain.Koc) string { return orDefault(k.ProductName, "sản phẩm mẫu") }

// ShipmentDraft tells the creator the sample is on its way.
func ShipmentDraft(k *domain.Koc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chào %s, bên mình đã gửi mẫu %s cho bạn rồi nha.", kocName(k), sampleName(k))
	where := ""
	if a := strings.TrimSpace(k.ShippingAddress); a != "" {
		where = " về địa chỉ " + a
	}
	switch {
	case k.ShippingCarrier == "Tự vận chuyển":
		fmt.Fprintf(&b, " Mẫu bên mình tự vận chuyển%s. Bạn để ý điện thoại giúp mình, nhận được hàng thì nhắn lại cho mình một tiếng nhé.", where)
	case k.TrackingCode != "":
		fmt.Fprintf(&b, " Mẫu đi qua J&T Express, mã vận đơn là %s, giao%s. Bạn để ý điện thoại giúp bên mình, shipper sẽ gọi trước khi giao nhé.", k.TrackingCode, orDefault(where, " tới bạn"))
	default:
		fmt.Fprintf(&b, " Mẫu đi qua J&T Express%s, bên mình sẽ gửi mã vận đơn cho bạn ngay khi có nhé.", where)
	}
	return b.String()
}

// DeliveredDraft tells the creator the sample arrived and asks for the video.
func DeliveredDraft(k *domain.Koc) string {
	return fmt.Sprintf("Chào %s, mẫu %s đã giao tới bạn rồi nha. Bạn trải nghiệm sản phẩm và lên video theo brief giúp bên mình nhé. Cần hỗ trợ gì bạn cứ nhắn lại cho mình.", kocName(k), sampleName(k))
}

// FollowUpDraft is reminder 1 or 2 of the outreach chain.
func FollowUpDraft(k *domain.Koc, step int) string {
	switch step {
	case 1:
		return fmt.Sprintf("Chào %s, cách đây 2 ngày bên mình có gửi lời mời hợp tác trải nghiệm sản phẩm %s. Bạn kiểm tra thông tin và phản hồi giúp nhãn hàng nhé!", kocName(k), sampleName(k))
	case 2:
		return fmt.Sprintf("Chào %s, bên mình nhắn ngắn hỏi thăm về lời mời trải nghiệm %s hôm trước ạ. Nếu đợt này lịch kênh chưa phù hợp hoặc sản phẩm không phù hợp thì bạn báo giúp bên mình một tiếng để bên mình ghi nhận nhé!", kocName(k), sampleName(k))
	}
	return ""
}

// VideoReminderDraft is video reminder 1 to 3.
func VideoReminderDraft(k *domain.Koc, step int) string {
	switch step {
	case 1:
		return fmt.Sprintf("Chào %s, bạn nhận được %s được 5 ngày rồi ạ. Gợi ý nhỏ cho bạn: với tệp kênh @%s, bạn có thể dựng bối cảnh phối đồ hoặc unbox nhanh 15 đến 30 giây rồi gắn giỏ hàng, video sẽ dễ được đề xuất hơn. Bạn dự kiến hôm nào lên bài để bên mình chuẩn bị đẩy traffic nhé!", kocName(k), sampleName(k), k.TiktokID)
	case 2:
		return fmt.Sprintf("Chào %s, bên mình hỏi thăm tiến độ quay dựng video cho %s ạ. Nếu bạn cần thêm thông tin về sản phẩm hoặc gặp vấn đề gì khi trải nghiệm thì nhắn lại để bên mình hỗ trợ ngay nhé!", kocName(k), sampleName(k))
	case 3:
		return fmt.Sprintf("Chào %s, đơn mẫu %s đã giao được 12 ngày nhưng bên mình chưa thấy video trên kênh. Bạn cho bên mình biết lịch đăng bài cụ thể giúp nhé, hoặc nếu sản phẩm không hợp thì bạn báo lại để bên mình hướng dẫn gửi hoàn mẫu.", kocName(k), sampleName(k))
	}
	return ""
}

// SelfShipQuestion asks whether a sample that the shop shipped by itself has arrived.
func SelfShipQuestion(k *domain.Koc) string {
	return fmt.Sprintf("Chào %s, mẫu bên mình tự vận chuyển gửi đi được 3 ngày rồi ạ. Bạn đã nhận được hàng chưa? Nhận được rồi thì bạn nhắn lại cho mình một tiếng nhé!", kocName(k))
}

// ReturnReminderDraft asks a creator whose sample did not suit to send it back.
func ReturnReminderDraft(k *domain.Koc) string {
	return fmt.Sprintf("Chào %s, mẫu không phù hợp bạn đã gửi lại giúp shop chưa ạ? Bạn cho mình xin mã vận đơn gửi hoàn nhé!", kocName(k))
}

// SampleApprovedDraft tells the creator the sample was approved and asks for, or confirms, the delivery data.
func SampleApprovedDraft(k *domain.Koc, product string) string {
	product = orDefault(strings.TrimSpace(product), sampleName(k))
	if a := strings.TrimSpace(k.ShippingAddress); a != "" && strings.TrimSpace(k.Phone) != "" {
		return fmt.Sprintf("Chào %s, bên mình đã duyệt mẫu %s cho bạn rồi nha. Bên mình sẽ gửi về địa chỉ %s, số điện thoại %s. Bạn xem giúp mình thông tin đã đúng chưa nhé.", kocName(k), product, a, k.Phone)
	}
	return fmt.Sprintf("Chào %s, bên mình đã duyệt mẫu %s cho bạn rồi nha. Bạn gửi giúp mình tên người nhận, số điện thoại và địa chỉ nhận hàng để bên mình gửi mẫu sớm nhất cho bạn nhé.", kocName(k), product)
}
