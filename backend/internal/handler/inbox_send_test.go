package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// fakePersonalLine is the personal Zalo account: it is either logged in or not.
type fakePersonalLine struct {
	service.PersonalZalo
	connected bool
	sent      []string
}

func (f *fakePersonalLine) IsConnected(context.Context) bool { return f.connected }
func (f *fakePersonalLine) Send(_ context.Context, id, text string) error {
	f.sent = append(f.sent, id+": "+text)
	return nil
}

type fakeSendOutreach struct {
	InboxOutreach
	zalo *service.ZaloService
}

func (f *fakeSendOutreach) Zalo() *service.ZaloService { return f.zalo }

type fakeSendConvs struct {
	ConversationStore
	conv  *domain.Conversation
	saves int
}

func (f *fakeSendConvs) FindByID(_ context.Context, id string) (*domain.Conversation, error) {
	if f.conv != nil && f.conv.ID.Hex() == id {
		return f.conv, nil
	}
	return nil, nil
}
func (f *fakeSendConvs) Save(context.Context, *domain.Conversation) error { f.saves++; return nil }

type fakeSendMsgs struct {
	MessageStore
	created []*domain.Message
}

func (f *fakeSendMsgs) Create(_ context.Context, m *domain.Message) error {
	f.created = append(f.created, m)
	return nil
}
func (f *fakeSendMsgs) Save(context.Context, *domain.Message) error { return nil }

// sendZaloApp wires the send route to a KOC thread that already knows its Zalo id (it was messaged before).
func sendZaloApp(line *fakePersonalLine) (*fiber.App, *domain.Conversation, *fakeSendMsgs) {
	conv := &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ContactPhone: "0327637213"}
	ms := &fakeSendMsgs{}
	zalo := service.NewZaloService(config.Config{}, line, nil) // no official-account token
	h := NewInboxHandler(nil, &fakeSendConvs{conv: conv}, ms, &fakeSendOutreach{zalo: zalo}, nil)
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/conversations/:conversationId/messages", h.SendMessage)
	return app, conv, ms
}

func TestAZaloMessageIsNotCalledSentWhenNoZaloLineCouldCarryIt(t *testing.T) {
	line := &fakePersonalLine{connected: false} // the bridge is down or logged out, and no OA token is set
	app, conv, _ := sendZaloApp(line)

	code, body := postJSON(app, "/conversations/"+conv.ID.Hex()+"/messages", `{"text":"Chào Linh"}`)
	if code != 201 {
		t.Fatalf("code %d %v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["status"] == domain.MessageSent {
		t.Fatalf("nothing left through Zalo, so the message must not be marked sent: %v", data)
	}
	if notice, _ := body["notice"].(string); strings.Contains(notice, "Đã gửi qua Zalo") {
		t.Fatalf("the answer must not claim the message was sent: %q", notice)
	}
	if len(line.sent) != 0 {
		t.Fatalf("no line was available, yet %v was sent", line.sent)
	}
}

func TestAZaloMessageIsSentWhenThePersonalLineIsUp(t *testing.T) {
	line := &fakePersonalLine{connected: true}
	app, conv, _ := sendZaloApp(line)

	code, body := postJSON(app, "/conversations/"+conv.ID.Hex()+"/messages", `{"text":"Chào Linh"}`)
	if code != 201 {
		t.Fatalf("code %d %v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["status"] != domain.MessageSent || body["notice"] != "Đã gửi qua Zalo." {
		t.Fatalf("a delivered message is sent: %v %v", data, body["notice"])
	}
	if len(line.sent) != 1 || line.sent[0] != "42: Chào Linh" {
		t.Fatalf("the text must reach the KOC's Zalo id: %v", line.sent)
	}
}
