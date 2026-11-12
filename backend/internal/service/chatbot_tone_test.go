package service

import (
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Every reply the chatbot can choose, over every intent and a few states of the chat, must read like a person wrote it.
func TestEveryChatbotReplyReadsLikeAPersonWroteIt(t *testing.T) {
	intents := []string{"AGREE_NO_ADDRESS", "AGREED", "ASK_POLICY", "ASK_CAST", "NEGOTIATE_CAST", "DECLINE", "NOT_FIT", "CONFIRMED_RECEIVED", "WILL_POST", "NUDGE", "GREETING", "OTHER", "VIDEO_LINK", "ADDRESS"}
	for _, in := range intents {
		for _, hasAddr := range []bool{false, true} {
			for _, text := range []string{"ok", "mình đồng ý", "hoa hồng bao nhiêu", "12 ngõ 5 Láng Hà Nội 0976451571", "alo", "không hợp", "đã nhận hàng", "https://vt.tiktok.com/ZS123/"} {
				d := dialog{Intent: in, Text: text, Name: "Linh", TiktokID: "linh", Rate: 10, CastFee: 300000, Phone: "0976451571"}
				if hasAddr {
					d.HadAddress, d.SameAddress = true, true
				}
				reply, _ := chooseReply(d)
				if i := strings.IndexAny(reply, "\"“”()[]{}"); reply != "" && i >= 0 {
					t.Errorf("intent %s: %q in %s", in, reply[i:i+1], reply)
				}
			}
		}
	}
	_ = domain.NewKoc
}
