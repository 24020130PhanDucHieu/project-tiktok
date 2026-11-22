package handler

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// The inbound route must file, link and answer a message under the Zalo account it arrived on.

type acctNotes struct {
	upsertAcct, linkAcct, botAcct, sendAcct []string
}

type acctConvs struct{ n *acctNotes }

func (a acctConvs) UpsertZaloInbound(ctx context.Context, _, _, _ string, _ time.Time) (*domain.Conversation, error) {
	a.n.upsertAcct = append(a.n.upsertAcct, domain.ZaloAccountFrom(ctx))
	return &domain.Conversation{ID: domain.NewID()}, nil
}

type acctBot struct{ n *acctNotes }

func (a acctBot) LinkZaloContact(ctx context.Context, _ string) error {
	a.n.linkAcct = append(a.n.linkAcct, domain.ZaloAccountFrom(ctx))
	return nil
}
func (a acctBot) ProcessInboundWithChatbot(ctx context.Context, _ *domain.Conversation, _ string, _ bool) (*service.InboundResult, error) {
	a.n.botAcct = append(a.n.botAcct, domain.ZaloAccountFrom(ctx))
	return &service.InboundResult{BotMessage: &domain.Message{Text: "ok"}}, nil
}

type acctBridge struct {
	ZaloBridge
	n *acctNotes
}

func (a acctBridge) Send(ctx context.Context, _, _ string) error {
	a.n.sendAcct = append(a.n.sendAcct, domain.ZaloAccountFrom(ctx))
	return nil
}

type defaultAccount struct{ id domain.ID }

func (d defaultAccount) Default(context.Context) (*domain.ZaloAccount, error) {
	return &domain.ZaloAccount{ID: d.id}, nil
}

func inboundNotes(t *testing.T, accounts service.ZaloDefaultAccount, body string) *acctNotes {
	t.Helper()
	n := &acctNotes{}
	h := NewZaloHandler(acctBridge{n: n}, acctConvs{n}, &fakeMsgCreator{}, acctBot{n}, &fakeAudit{})
	if accounts != nil {
		h.WithAccounts(accounts)
	}
	if code, _ := postJSON(inboundApp(h), "/inbound", body); code != 200 {
		t.Fatalf("inbound answered %d", code)
	}
	return n
}

func all(t *testing.T, got []string, want string) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%q]", got, want)
	}
}

func TestAMessageIsFiledLinkedAndAnsweredUnderTheAccountItArrivedOn(t *testing.T) {
	n := inboundNotes(t, nil, `{"accountId":"acc-B","externalContactId":"u1","text":"hi"}`)
	all(t, n.upsertAcct, "acc-B")
	all(t, n.linkAcct, "acc-B")
	all(t, n.botAcct, "acc-B")
	all(t, n.sendAcct, "acc-B")
}

func TestTheOldBridgeKeyMeansTheDefaultAccount(t *testing.T) {
	id := domain.NewID()
	for _, body := range []string{
		`{"accountId":"default","externalContactId":"u1","text":"hi"}`,
		`{"externalContactId":"u1","text":"hi"}`,
	} {
		n := inboundNotes(t, defaultAccount{id}, body)
		all(t, n.upsertAcct, id.Hex())
		all(t, n.sendAcct, id.Hex())
	}
}

func TestWithoutStoredAccountsAnInboundMessageNamesNoAccount(t *testing.T) {
	n := inboundNotes(t, nil, `{"externalContactId":"u1","text":"hi"}`)
	all(t, n.upsertAcct, "")
	all(t, n.sendAcct, "")
}

func inboundApp(h *ZaloHandler) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/inbound", h.ReceivePersonalInbound)
	return app
}
