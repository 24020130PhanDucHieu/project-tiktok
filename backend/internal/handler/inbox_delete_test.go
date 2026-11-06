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
