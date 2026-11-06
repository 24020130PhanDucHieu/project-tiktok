package service

import (
	"strings"
	"testing"
)

var allIntents = []string{"PROVIDED_ADDRESS", "AGREE_NO_ADDRESS", "NEGOTIATE_CAST", "ASK_POLICY", "CONFIRMED_RECEIVED",
	"SUBMITTED_VIDEO", "NOT_FIT_RETURN", "DECLINED", "WILL_POST"}

func TestEveryReplyReadsLikeAPersonNotLikeASystem(t *testing.T) {
	d := replyData{Name: "Linh", Rate: 12, CastFee: 500000, Phone: "0961999222"}
	banned := []string{"[", "]", "\"", "“", "”", "Chatbot", "chatbot", "Hệ thống", "hệ thống", "Booker", "booker", "Mã P0", "Đề xuất mã", "+ "}
	for _, intent := range allIntents {
		got := botText(intent, d)
		if got == "" {
			t.Fatalf("%s has no reply", intent)
		}
		for _, b := range banned {
			if strings.Contains(got, b) {
				t.Fatalf("%s: reply must not contain %q: %s", intent, b, got)
			}
		}
		if !strings.Contains(got, "nha") {
			t.Fatalf("%s: reply should sound like the shop (nha): %s", intent, got)
		}
	}
}

func TestNoReplyForIntentsTheBotDoesNotHandle(t *testing.T) {
	for _, intent := range []string{"GENERAL_REPLY", "SET_DNC", ""} {
		if got := botText(intent, replyData{Name: "x"}); got != "" {
			t.Fatalf("%q should stay silent, got %s", intent, got)
		}
	}
}

func TestTheKocIsCalledByNameOnlyWhenWeKnowIt(t *testing.T) {
	if got := botText("AGREE_NO_ADDRESS", replyData{Name: "Linh"}); !strings.Contains(got, "Cảm ơn Linh nha") {
		t.Fatalf("a real name is used: %s", got)
	}
	for _, unknown := range []string{"", "  ", "KOC @user47icgazh1k"} {
		got := botText("AGREE_NO_ADDRESS", replyData{Name: unknown})
		if !strings.Contains(got, "Cảm ơn bạn nha") || strings.Contains(got, "KOC @") {
			t.Fatalf("%q must become bạn: %s", unknown, got)
		}
	}
}

func TestThePolicyReplyUsesTheRealCastFeeAndRate(t *testing.T) {
	withFee := botText("ASK_POLICY", replyData{CastFee: 500000, Rate: 12})
	if !strings.Contains(withFee, "Phí cast bên mình duyệt là 500.000đ") || !strings.Contains(withFee, "12%") {
		t.Fatalf("%s", withFee)
	}
	noFee := botText("ASK_POLICY", replyData{})
	if !strings.Contains(noFee, "gửi mẫu cho bạn trải nghiệm trước") || !strings.Contains(noFee, "10%") {
		t.Fatalf("default rate is 10%% and no fee talk without a fee: %s", noFee)
	}
}

func TestTheAddressReplyMentionsThePhoneOnlyWhenGiven(t *testing.T) {
	if got := botText("PROVIDED_ADDRESS", replyData{Name: "A", Phone: "0961999222"}); !strings.Contains(got, "0961999222") {
		t.Fatal(got)
	}
	if got := botText("PROVIDED_ADDRESS", replyData{Name: "A"}); strings.Contains(got, "điện thoại số") {
		t.Fatal(got)
	}
}

func TestThePreviewAndTheDoNotDisturbRule(t *testing.T) {
	p := ParseKocInboundText("ok mình đồng ý hợp tác nha")
	if got := BuildChatbotAutoReply(ChatbotKocInfo{DisplayName: "Linh"}, p); !strings.Contains(got, "Linh") {
		t.Fatalf("preview should use the shared wording: %s", got)
	}
	if got := BuildChatbotAutoReply(ChatbotKocInfo{DisplayName: "Linh", Dnc: true}, p); got != "" {
		t.Fatalf("a do-not-disturb KOC gets no reply: %s", got)
	}
}
