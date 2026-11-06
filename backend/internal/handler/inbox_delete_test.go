package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
)

type fakeDelConvs struct {
	ConversationStore
	conv    *domain.Conversation
	deleted []primitive.ObjectID
	hidden  []primitive.ObjectID
}

func (f *fakeDelConvs) FindByID(_ context.Context, id string) (*domain.Conversation, error) {
	if f.conv != nil && f.conv.ID.Hex() == id {
		return f.conv, nil
	}
	return nil, nil
}
func (f *fakeDelConvs) Delete(_ context.Context, id primitive.ObjectID) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeDelMsgs struct {
	MessageStore
	cleared []primitive.ObjectID
}

func (f *fakeDelMsgs) DeleteByConversation(_ context.Context, id primitive.ObjectID) error {
	f.cleared = append(f.cleared, id)
	return nil
}

func deleteApp(cv *fakeDelConvs, ms *fakeDelMsgs) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Delete("/conversations/:conversationId", NewInboxHandler(nil, cv, ms, nil, nil).DeleteConversation)
	return app
}

func TestDeletingAConversationRemovesItAndItsMessagesForGood(t *testing.T) {
	conv := &domain.Conversation{ID: primitive.NewObjectID(), ContactName: "Bùi Đức Việt"}
	cv, ms := &fakeDelConvs{conv: conv}, &fakeDelMsgs{}
	resp, err := deleteApp(cv, ms).Test(httptest.NewRequest("DELETE", "/conversations/"+conv.ID.Hex(), nil), 10000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status %v err %v", resp, err)
	}
	if len(cv.deleted) != 1 || cv.deleted[0] != conv.ID || len(ms.cleared) != 1 || ms.cleared[0] != conv.ID {
		t.Fatalf("the conversation and its messages must both go: conv %v msgs %v", cv.deleted, ms.cleared)
	}
	if len(cv.hidden) != 0 {
		t.Fatal("deleting is not hiding")
	}
}

func TestDeletingAnUnknownConversationIsNotFound(t *testing.T) {
	cv, ms := &fakeDelConvs{}, &fakeDelMsgs{}
	resp, _ := deleteApp(cv, ms).Test(httptest.NewRequest("DELETE", "/conversations/"+primitive.NewObjectID().Hex(), nil), 10000)
	if resp.StatusCode != 404 || len(cv.deleted) != 0 || len(ms.cleared) != 0 {
		t.Fatalf("status %d, nothing may be removed", resp.StatusCode)
	}
}

type fakeOpenKocs struct {
	InboxKocStore
	koc *domain.Koc
}

func (f *fakeOpenKocs) FindActiveByID(context.Context, string) (*domain.Koc, error) {
	return f.koc, nil
}

type fakeOpenConvs struct {
	ConversationStore
	existing *domain.Conversation
	ensured  []domain.KocContact
	unhidden []primitive.ObjectID
}

func (f *fakeOpenConvs) EnsureKocContact(_ context.Context, c domain.KocContact) (*domain.Conversation, error) {
	f.ensured = append(f.ensured, c)
	if f.existing != nil {
		return f.existing, nil
	}
	return &domain.Conversation{ID: primitive.NewObjectID(), ContactHandle: c.Handle, ContactName: c.Name}, nil
}
func (f *fakeOpenConvs) SetHidden(_ context.Context, id primitive.ObjectID, hidden bool) error {
	if !hidden {
		f.unhidden = append(f.unhidden, id)
	}
	return nil
}

func openApp(k *fakeOpenKocs, cv *fakeOpenConvs) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/kocs/:kocId/conversation", NewInboxHandler(k, cv, nil, nil, nil).OpenKocConversation)
	return app
}

func TestOpeningAKocChatCreatesItWhenItWasDeleted(t *testing.T) {
	k := &fakeOpenKocs{koc: &domain.Koc{TiktokID: "giadinhtraicay1202", DisplayName: "Gia đình trái cây", Phone: "0327637213"}}
	cv := &fakeOpenConvs{}
	code, body := postJSON(openApp(k, cv), "/kocs/abc/conversation", ``)
	if code != 200 || len(cv.ensured) != 1 || cv.ensured[0].Handle != "@giadinhtraicay1202" || cv.ensured[0].Phone != "0327637213" {
		t.Fatalf("code %d body %v ensured %+v", code, body, cv.ensured)
	}
}

func TestOpeningAHiddenKocChatBringsItBack(t *testing.T) {
	k := &fakeOpenKocs{koc: &domain.Koc{TiktokID: "x", DisplayName: "X"}}
	cv := &fakeOpenConvs{existing: &domain.Conversation{ID: primitive.NewObjectID(), Hidden: true}}
	if code, _ := postJSON(openApp(k, cv), "/kocs/abc/conversation", ``); code != 200 || len(cv.unhidden) != 1 {
		t.Fatalf("code %d unhidden %v", code, cv.unhidden)
	}
}

func TestOpeningTheChatOfAnUnknownKocIsNotFound(t *testing.T) {
	if code, _ := postJSON(openApp(&fakeOpenKocs{}, &fakeOpenConvs{}), "/kocs/abc/conversation", ``); code != 404 {
		t.Fatalf("code %d", code)
	}
}
