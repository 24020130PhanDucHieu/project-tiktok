package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ---- fakes: the outreach service only sees the ports of ports.go ----

type fakeOutreachKocs struct {
	OutreachKocs
	koc   *domain.Koc
	saves int
}

func (f *fakeOutreachKocs) FindActiveByContact(context.Context, string, ...string) (*domain.Koc, error) {
	return f.koc, nil
}
func (f *fakeOutreachKocs) Save(_ context.Context, k *domain.Koc) error { f.saves++; return nil }

type fakeOutreachConvs struct {
	OutreachConvs
	saved []string
}

func (f *fakeOutreachConvs) Save(_ context.Context, c *domain.Conversation) error {
	f.saved = append(f.saved, c.LastMessage)
	return nil
}

type fakeOutreachMsgs struct {
	OutreachMsgs
	all []domain.Message // oldest first
}

func (f *fakeOutreachMsgs) Create(_ context.Context, m *domain.Message) error {
	m.SentAt = time.Now()
	f.all = append(f.all, *m)
	return nil
}
func (f *fakeOutreachMsgs) CountProactiveSince(_ context.Context, since time.Time) (int64, error) {
	var n int64
	for _, m := range f.all {
		if m.Proactive && !m.SentAt.Before(since) {
			n++
		}
	}
	return n, nil
}
func (f *fakeOutreachMsgs) Recent(_ context.Context, _ domain.ID, n int64) ([]domain.Message, error) {
	var out []domain.Message
	for i := len(f.all) - 1; i >= 0 && int64(len(out)) < n; i-- {
		out = append(out, f.all[i])
	}
	return out, nil
}

type chat struct {
	t    *testing.T
	svc  *OutreachService
	koc  *domain.Koc
	conv *domain.Conversation
	msgs *fakeOutreachMsgs
}

func newChat(t *testing.T, stage string) *chat {
	t.Helper()
	koc := domain.NewKoc()
	koc.TiktokID, koc.DisplayName, koc.Stage, koc.Status = "giadinhtraicay1202", "giadinhtraicay1202", stage, domain.KocActive
	msgs := &fakeOutreachMsgs{}
	svc := &OutreachService{kocs: &fakeOutreachKocs{koc: koc}, convs: &fakeOutreachConvs{}, msgs: msgs}
	return &chat{t: t, svc: svc, koc: koc, conv: &domain.Conversation{ID: domain.NewID(), ContactHandle: "@giadinhtraicay1202"}, msgs: msgs}
}

// say plays one message of the KOC and returns what the chatbot answered ("" when it stayed silent).
func (c *chat) say(text string, autoReply bool) string {
	c.t.Helper()
	c.msgs.all = append(c.msgs.all, domain.Message{ConversationID: c.conv.ID, Direction: domain.DirectionInbound, Text: text})
	res, err := c.svc.ProcessInboundWithChatbot(context.Background(), c.conv, text, autoReply)
	if err != nil {
		c.t.Fatal(err)
	}
	if res.BotMessage == nil {
		return ""
	}
	return res.BotMessage.Text
}

func TestTheChatbotFollowsTheConversationAndDoesNotRepeatItself(t *testing.T) {
	c := newChat(t, domain.StageContacted)

	if got := c.say("mình đồng ý hợp tác nha", true); !strings.Contains(got, "xin tên người nhận") || c.koc.Stage != domain.StageReplied {
		t.Fatalf("agreeing asks for the delivery data and moves the KOC to REPLIED (%s): %s", c.koc.Stage, got)
	}

	got := c.say("địa chỉ mình ở 12 ngõ 5 đường Láng, Đống Đa, Hà Nội, 0976451571", true)
	if !strings.Contains(got, "nhận được thông tin rồi") || !strings.Contains(got, "0976451571") {
		t.Fatalf("the address is acknowledged: %s", got)
	}
	if c.koc.Stage != domain.StageAgreed || c.koc.ShippingAddress == "" {
		t.Fatalf("address saved and stage AGREED: %s %q", c.koc.Stage, c.koc.ShippingAddress)
	}

	policy := c.say("hoa hồng bên mình là bao nhiêu vậy", true)
	if strings.Contains(policy, "gửi mình số điện thoại") || !strings.Contains(policy, "có rồi") {
		t.Fatalf("it already has the address and must not ask for it again: %s", policy)
	}
	if c.koc.Stage != domain.StageAgreed {
		t.Fatalf("asking about the policy must not move the KOC back (%s)", c.koc.Stage)
	}

	again := c.say("hoa hồng là bao nhiêu thế", true)
	if again == "" || len(again) >= len(policy) || !strings.Contains(again, "như mình nhắn ở trên") {
		t.Fatalf("a repeated question gets a short answer: %q", again)
	}

	if agree := c.say("ok bạn", true); !strings.Contains(agree, "có rồi") || strings.Contains(agree, "xin tên người nhận") {
		t.Fatalf("agreeing again after the address was given must not ask again: %s", agree)
	}
	if c.koc.Stage != domain.StageAgreed {
		t.Fatalf("agreeing again must not move the KOC back (%s)", c.koc.Stage)
	}

	if hi := c.say("alo bạn ơi", true); !strings.Contains(hi, "Mình đây nha") {
		t.Fatalf("a nudge is answered: %s", hi)
	}
	if hi := c.say("alo", true); hi != "" {
		t.Fatalf("a second nudge in a row is not chased: %s", hi)
	}
	if small := c.say("mình khỏe còn bạn", true); small != "" {
		t.Fatalf("small talk is not answered by the bot: %s", small)
	}
}

func TestWithTheChatbotOffTheStateStillMovesButNothingIsSaid(t *testing.T) {
	c := newChat(t, domain.StageContacted)
	if got := c.say("mình đồng ý hợp tác nha", false); got != "" {
		t.Fatalf("no bot message while the chatbot is off: %s", got)
	}
	if c.koc.Stage != domain.StageReplied {
		t.Fatalf("the KOC's progress is still recorded: %s", c.koc.Stage)
	}
	for _, m := range c.msgs.all {
		if m.Direction == domain.DirectionOutbound {
			t.Fatal("nothing outbound may be stored")
		}
	}
}

func TestADoNotDisturbKocIsNeverAnswered(t *testing.T) {
	c := newChat(t, domain.StageContacted)
	c.koc.Dnc = true
	if got := c.say("mình đồng ý", true); got != "" {
		t.Fatalf("%s", got)
	}
	if got := newChat(t, domain.StageContacted).say("đừng nhắn nữa", true); got != "" {
		t.Fatalf("asking to stop gets silence: %s", got)
	}
}

func TestEveryBotReplyRemembersWhatItAnswered(t *testing.T) {
	c := newChat(t, domain.StageContacted)
	c.say("mình đồng ý hợp tác nha", true)
	var last domain.Message
	for _, m := range c.msgs.all {
		if m.Direction == domain.DirectionOutbound {
			last = m
		}
	}
	if last.Intent != "AGREE_NO_ADDRESS" {
		t.Fatalf("the stored bot message carries its intent (its memory): %q", last.Intent)
	}
}
