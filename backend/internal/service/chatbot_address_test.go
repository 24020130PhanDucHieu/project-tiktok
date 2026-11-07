package service

import "testing"

// A KOC message is taken for a delivery address only on real evidence. One accidental word used to be enough:
// "ấp" sits inside "thấp", so even the phrase the negotiation rule is built on ("hoa hồng thấp quá") became an address
// and moved the KOC to AGREED, ready for a sample it had never accepted.
func TestAKocMessageIsOnlyAnAddressWhenItLooksLikeOne(t *testing.T) {
	notAddress := []struct{ text, intent string }{
		{"hoa hồng thấp quá", "NEGOTIATE_CAST"},
		{"giá thấp quá mình xin phép không nhận", "DECLINED"},
		{"xã hội này khó thật", "GENERAL_REPLY"},
		{"gửi mình đường link brief nhé", "GENERAL_REPLY"},
		{"mình tỉnh táo rồi, ok shop", "AGREE_NO_ADDRESS"},
		{"tòa soạn có nhận review không", "GENERAL_REPLY"},
		{"mình ở hà nội thôi", "GENERAL_REPLY"}, // a city alone is not an address to ship to
	}
	for _, c := range notAddress {
		got := ParseKocInboundText(c.text)
		if got.Intent != c.intent || got.ShippingAddress != "" {
			t.Errorf("%q: intent %s address %q, want %s with no address", c.text, got.Intent, got.ShippingAddress, c.intent)
		}
	}
}

func TestRealAddressesAreStillRecognised(t *testing.T) {
	for _, text := range []string{
		"địa chỉ mình ở 12 ngõ 5 đường Láng, Đống Đa, Hà Nội, 0976451571",
		"Địa chỉ nhận mẫu: Ký túc xá Ngoại ngữ VNU, Cầu Giấy, Hà Nội - SĐT 0988123456",
		"số nhà 5, ngõ 12 Cầu Giấy",
		"Nguyễn Văn A, 0976451571, 15 Thanh Xuân, Hà Nội",
		"xã Đông Hòa, huyện Đông Anh, Hà Nội",
		"Ký túc xá B3 ĐH Bách Khoa",
		"12 đường Láng",
		"Thôn 3, xã Hòa Bình",
		"ĐC: 45 Lê Lợi, quận 1, TP.HCM",
	} {
		got := ParseKocInboundText(text)
		if got.Intent != "PROVIDED_ADDRESS" || got.ShippingAddress == "" {
			t.Errorf("%q must still be an address, got intent %s", text, got.Intent)
		}
	}
}
