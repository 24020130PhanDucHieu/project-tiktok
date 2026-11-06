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

func reply(t *testing.T, d dialog) (string, string) {
	t.Helper()
	return chooseReply(d)
}

func TestItDoesNotAskForTheAddressAgainOnceItHasIt(t *testing.T) {
	askedBefore := []string{"AGREE_NO_ADDRESS"}
	got, _ := reply(t, dialog{Intent: "AGREE_NO_ADDRESS", Name: "Linh", HadAddress: true, Recent: askedBefore, Stage: "AGREED"})
	if strings.Contains(got, "xin tên người nhận") || strings.Contains(got, "gửi mình") || !strings.Contains(got, "có rồi") {
		t.Fatalf("it must say it already has the information: %s", got)
	}
	policy, _ := reply(t, dialog{Intent: "ASK_POLICY", HadAddress: true})
	if strings.Contains(policy, "gửi mình số điện thoại") || !strings.Contains(policy, "có rồi") {
		t.Fatalf("the policy answer must not ask for the address again: %s", policy)
	}
	fresh, _ := reply(t, dialog{Intent: "ASK_POLICY"})
	if !strings.Contains(fresh, "gửi mình số điện thoại với địa chỉ") {
		t.Fatalf("without an address it still asks for it: %s", fresh)
	}
}

func TestItDoesNotRepeatItselfWhenAskedTheSameThingAgain(t *testing.T) {
	first, _ := reply(t, dialog{Intent: "ASK_POLICY", Rate: 12})
	again, _ := reply(t, dialog{Intent: "ASK_POLICY", Rate: 12, Recent: []string{"ASK_POLICY"}, LastBotText: first})
	if again == "" || again == first || len(again) >= len(first) || !strings.Contains(again, "12%") {
		t.Fatalf("the second answer must be shorter and different: %q", again)
	}
	waiting, _ := reply(t, dialog{Intent: "AGREE_NO_ADDRESS", Recent: []string{"AGREE_NO_ADDRESS"}})
	if !strings.Contains(waiting, "đang chờ") {
		t.Fatalf("a second agreement without data says it is waiting: %s", waiting)
	}
}

func TestTheSameAddressOrVideoIsRecognised(t *testing.T) {
	same, _ := reply(t, dialog{Intent: "PROVIDED_ADDRESS", HadAddress: true, SameAddress: true})
	if !strings.Contains(same, "có rồi") {
		t.Fatalf("%s", same)
	}
	changed, _ := reply(t, dialog{Intent: "PROVIDED_ADDRESS", HadAddress: true})
	if !strings.Contains(changed, "cập nhật địa chỉ mới") {
		t.Fatalf("%s", changed)
	}
	video, _ := reply(t, dialog{Intent: "SUBMITTED_VIDEO", SameVideo: true})
	if !strings.Contains(video, "ghi nhận rồi") {
		t.Fatalf("%s", video)
	}
	got, _ := reply(t, dialog{Intent: "CONFIRMED_RECEIVED", Stage: "DELIVERED"})
	if strings.Contains(got, "dùng thử rồi lên video") {
		t.Fatalf("an already delivered KOC is not told to start over: %s", got)
	}
}

func TestAGreetingGetsAnAnswerThatKnowsWhatIsStillNeeded(t *testing.T) {
	waiting, intent := reply(t, dialog{Intent: "GENERAL_REPLY", Text: "alo bạn ơi", Recent: []string{"AGREE_NO_ADDRESS"}})
	if intent != "NUDGE" || !strings.Contains(waiting, "địa chỉ nhận hàng") {
		t.Fatalf("%q %q", intent, waiting)
	}
	generic, _ := reply(t, dialog{Intent: "GENERAL_REPLY", Text: "hello"})
	if !strings.Contains(generic, "Mình đây nha") || strings.Contains(generic, "địa chỉ") {
		t.Fatalf("nothing pending, so no chasing: %s", generic)
	}
	chased, _ := reply(t, dialog{Intent: "GENERAL_REPLY", Text: "alo", Recent: []string{"NUDGE"}})
	if chased != "" {
		t.Fatalf("one nudge answer is enough: %s", chased)
	}
	if silent, _ := reply(t, dialog{Intent: "GENERAL_REPLY", Text: "mình khỏe còn bạn"}); silent != "" {
		t.Fatalf("small talk is not answered: %s", silent)
	}
}

func TestTheSameMessageIsNeverSentTwiceInARow(t *testing.T) {
	text, _ := reply(t, dialog{Intent: "WILL_POST"})
	if again, _ := reply(t, dialog{Intent: "WILL_POST", LastBotText: text}); again != "" {
		t.Fatalf("identical text twice in a row: %s", again)
	}
}

func TestAHandleIsNotUsedAsAName(t *testing.T) {
	for _, n := range []string{"giadinhtraicay1202", "user_47", "KOC @x", "", "hoadoo.204"} {
		if got := callName(n, ""); got != "bạn" {
			t.Fatalf("%q must become bạn, got %s", n, got)
		}
	}
	if got := callName("Phan Đức Hiếu", "giadinhtraicay1202"); got != "Phan Đức Hiếu" {
		t.Fatal(got)
	}
	if got := callName("Linh", ""); got != "Linh" {
		t.Fatal(got)
	}
	if got := callName("giadinhtraicay", "giadinhtraicay"); got != "bạn" {
		t.Fatal(got)
	}
}
