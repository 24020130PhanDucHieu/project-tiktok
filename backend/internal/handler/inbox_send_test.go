package handler

import (
	"context"
	"strings"
	"testing"
	"time"

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
func (f *fakePersonalLine) ResolveByPhone(context.Context, string) (*service.ZaloUser, error) {
	return &service.ZaloUser{UserID: "77", DisplayName: "Linh Nguyen"}, nil
}
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
	saves int // whole-document saves: they replace fields another writer may have just changed
	// the field updates that must be used instead
	lastMessages []string
	zaloContacts []domain.ZaloContact
}

func (f *fakeSendConvs) FindByID(_ context.Context, id string) (*domain.Conversation, error) {
	if f.conv != nil && f.conv.ID.Hex() == id {
		return f.conv, nil
	}
	return nil, nil
}
func (f *fakeSendConvs) Save(context.Context, *domain.Conversation) error { f.saves++; return nil }
func (f *fakeSendConvs) SetLastMessageAt(_ context.Context, _ domain.ID, text string, _ time.Time) error {
	f.lastMessages = append(f.lastMessages, text)
	return nil
}
func (f *fakeSendConvs) SetZaloContact(_ context.Context, _ domain.ID, c domain.ZaloContact) error {
	f.zaloContacts = append(f.zaloContacts, c)
	return nil
}

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
	app, conv, ms, _ := sendZaloAppWith(line, &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ContactPhone: "0327637213"})
	return app, conv, ms
}

func sendZaloAppWith(line *fakePersonalLine, conv *domain.Conversation) (*fiber.App, *domain.Conversation, *fakeSendMsgs, *fakeSendConvs) {
	ms := &fakeSendMsgs{}
	convs := &fakeSendConvs{conv: conv}
	zalo := service.NewZaloService(config.Config{}, line, nil) // no official-account token
	h := NewInboxHandler(nil, convs, ms, &fakeSendOutreach{zalo: zalo}, nil)
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/conversations/:conversationId/messages", h.SendMessage)
	return app, conv, ms, convs
}

// Sending used to save the whole conversation back, which undid anything written to it since it was read: the unread
// count a KOC's message had just raised, or the preview of a newer message. Only the fields this step changes are written.
func TestSendingWritesOnlyThePreviewNotTheWholeConversation(t *testing.T) {
	app, conv, _, convs := sendZaloAppWith(&fakePersonalLine{connected: true},
		&domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ContactPhone: "0327637213"})

	if code, body := postJSON(app, "/conversations/"+conv.ID.Hex()+"/messages", `{"text":"Chào Linh"}`); code != 201 {
		t.Fatalf("%d %v", code, body)
	}
	if convs.saves != 0 {
		t.Fatalf("the whole conversation must not be replaced, %d saves", convs.saves)
	}
	if len(convs.lastMessages) != 1 || convs.lastMessages[0] != "Chào Linh" {
		t.Fatalf("the preview is updated on its own: %v", convs.lastMessages)
	}
}

func TestFindingTheZaloAccountOfANewKocWritesOnlyTheContactFields(t *testing.T) {
	app, conv, _, convs := sendZaloAppWith(&fakePersonalLine{connected: true},
		&domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelInternal, ContactPhone: "0988111222"}) // no Zalo id yet

	if code, body := postJSON(app, "/conversations/"+conv.ID.Hex()+"/messages", `{"text":"Chào"}`); code != 201 {
		t.Fatalf("%d %v", code, body)
	}
	if convs.saves != 0 {
		t.Fatalf("no whole-document save, got %d", convs.saves)
	}
	want := domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: "77", Phone: "0988111222", Name: "Linh Nguyen"}
	if len(convs.zaloContacts) != 1 || convs.zaloContacts[0] != want {
		t.Fatalf("the contact fields found by the lookup: %+v", convs.zaloContacts)
	}
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
