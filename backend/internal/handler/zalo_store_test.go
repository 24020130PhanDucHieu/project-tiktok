package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type fakeZaloBridge struct {
	ZaloBridge
	sent    []string
	sendErr error
}

func (f *fakeZaloBridge) Send(_ context.Context, userID, text string) error {
	f.sent = append(f.sent, userID+": "+text)
	return f.sendErr
}

type fakeZaloConvs struct{ upserts []string }

func (f *fakeZaloConvs) UpsertZaloInbound(_ context.Context, externalID, name, text string, _ time.Time) (*domain.Conversation, error) {
	f.upserts = append(f.upserts, externalID+"|"+name+"|"+text)
	return &domain.Conversation{ID: primitive.NewObjectID()}, nil
}

type fakeMsgCreator struct{ created []*domain.Message }

func (f *fakeMsgCreator) Create(_ context.Context, m *domain.Message) error {
	f.created = append(f.created, m)
	return nil
}

type fakeChatbot struct {
	reply  *domain.Message
	calls  int
	linked int
}

func (f *fakeChatbot) LinkZaloContact(context.Context, string) error { f.linked++; return nil }

func (f *fakeChatbot) ProcessInboundWithChatbot(context.Context, *domain.Conversation, string, bool) (*service.InboundResult, error) {
	f.calls++
	return &service.InboundResult{BotMessage: f.reply}, nil
}

func zaloApp(b *fakeZaloBridge, cv *fakeZaloConvs, ms *fakeMsgCreator, bot *fakeChatbot) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/inbound", NewZaloHandler(b, cv, ms, bot, &fakeAudit{}).ReceivePersonalInbound)
	return app
}

func TestAnInboundZaloMessageNeedsAContactAndAText(t *testing.T) {
	b, cv, ms, bot := &fakeZaloBridge{}, &fakeZaloConvs{}, &fakeMsgCreator{}, &fakeChatbot{}
	for _, body := range []string{`{}`, `{"externalContactId":"u1"}`, `{"text":"hi"}`} {
		if code, _ := postJSON(zaloApp(b, cv, ms, bot), "/inbound", body); code != 400 {
			t.Fatalf("%s must be refused, got %d", body, code)
		}
	}
	if len(cv.upserts) != 0 || len(ms.created) != 0 || bot.calls != 0 {
		t.Fatal("nothing may be stored or answered for an invalid inbound message")
	}
}

func TestAnInboundZaloMessageIsStoredAnsweredAndTheReplyIsSentBack(t *testing.T) {
	b, cv, ms := &fakeZaloBridge{}, &fakeZaloConvs{}, &fakeMsgCreator{}
	bot := &fakeChatbot{reply: &domain.Message{Text: "Cảm ơn bạn, shop đã nhận"}}
	code, body := postJSON(zaloApp(b, cv, ms, bot), "/inbound", `{"externalContactId":"u1","name":"Anh","text":"Xin chào"}`)
	if code != 200 || body["success"] != true {
		t.Fatalf("%d %v", code, body)
	}
	if len(cv.upserts) != 1 || cv.upserts[0] != "u1|Anh|Xin chào" {
		t.Fatalf("conversation: %v", cv.upserts)
	}
	if len(ms.created) != 1 || ms.created[0].Direction != domain.DirectionInbound || ms.created[0].Text != "Xin chào" {
		t.Fatalf("the inbound message must be stored: %+v", ms.created)
	}
	if bot.calls != 1 || len(b.sent) != 1 || b.sent[0] != "u1: Cảm ơn bạn, shop đã nhận" {
		t.Fatalf("the bot reply must go back through Zalo: calls %d sent %v", bot.calls, b.sent)
	}
}

func TestAFailedBotReplyDoesNotFailTheInboundIngestion(t *testing.T) {
	b := &fakeZaloBridge{sendErr: errors.New("bridge down")}
	cv, ms := &fakeZaloConvs{}, &fakeMsgCreator{}
	bot := &fakeChatbot{reply: &domain.Message{Text: "ok"}}
	code, _ := postJSON(zaloApp(b, cv, ms, bot), "/inbound", `{"externalContactId":"u1","text":"hi"}`)
	if code != 200 || len(ms.created) != 1 {
		t.Fatalf("the message is stored even when the reply cannot be delivered: %d stored %d", code, len(ms.created))
	}
}

func TestAnInboundZaloMessageIsLinkedToItsKocBeforeItIsFiled(t *testing.T) {
	b, cv, ms, bot := &fakeZaloBridge{}, &fakeZaloConvs{}, &fakeMsgCreator{}, &fakeChatbot{}
	if code, _ := postJSON(zaloApp(b, cv, ms, bot), "/inbound", `{"externalContactId":"u1","text":"hi"}`); code != 200 {
		t.Fatalf("code %d", code)
	}
	if bot.linked != 1 {
		t.Fatalf("the sender must be matched to a KOC once per message, got %d", bot.linked)
	}
}
