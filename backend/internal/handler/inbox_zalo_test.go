package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// socialLine is a personal Zalo line that records the friend requests and reactions it was asked to make.
type socialLine struct {
	service.PersonalZalo
	connected bool
	friend    service.ZaloFriend
	sendErr   error
	requests  []string
	reacted   []string
}

func (f *socialLine) IsConnected(context.Context) bool { return f.connected }
func (f *socialLine) ResolveByPhone(context.Context, string) (*service.ZaloUser, error) {
	return &service.ZaloUser{UserID: "77", DisplayName: "Linh"}, nil
}
func (f *socialLine) FriendStatus(context.Context, string) (service.ZaloFriend, error) {
	return f.friend, nil
}
func (f *socialLine) SendFriendRequest(_ context.Context, id, note string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.requests = append(f.requests, id+": "+note)
	return nil
}
func (f *socialLine) React(_ context.Context, id, msgID, cliMsgID, code string) error {
	f.reacted = append(f.reacted, id+"/"+msgID+"/"+cliMsgID+"/"+code)
	return nil
}

type socialMsgs struct {
	MessageStore
	msg       *domain.Message
	created   []*domain.Message
	reactions []string
}

func (f *socialMsgs) Create(_ context.Context, m *domain.Message) error {
	f.created = append(f.created, m)
	return nil
}
func (f *socialMsgs) FindByID(_ context.Context, _ domain.ID, id string) (*domain.Message, error) {
	if f.msg != nil && f.msg.ID.Hex() == id {
		c := *f.msg
		return &c, nil
	}
	return nil, nil
}
func (f *socialMsgs) SetReaction(_ context.Context, _ domain.ID, _ string, key string) error {
	f.reactions = append(f.reactions, key)
	return nil
}
func (f *socialMsgs) ListByConversation(context.Context, string, string) ([]domain.Message, error) {
	return []domain.Message{*f.msg, {Direction: domain.DirectionOutbound, ZaloMsgID: "9", ZaloCliMsgID: "9"}}, nil
}

func socialApp(line *socialLine, conv *domain.Conversation, msg *domain.Message) (*fiber.App, *socialMsgs) {
	ms := &socialMsgs{msg: msg}
	h := NewInboxHandler(nil, &fakeSendConvs{conv: conv}, ms, &fakeSendOutreach{zalo: service.NewZaloService(config.Config{}, line, nil)}, nil)
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Get("/conversations/:conversationId/zalo/friend", h.ZaloFriend)
	app.Post("/conversations/:conversationId/zalo/friend-request", h.ZaloFriendRequest)
	app.Put("/conversations/:conversationId/messages/:messageId/reaction", h.ReactToMessage)
	app.Get("/conversations/:conversationId/messages", h.ListMessages)
	return app, ms
}

func socialCall(app *fiber.App, method, path, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 10000)
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func kocThread() *domain.Conversation {
	return &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ContactPhone: "0327637213"}
}

func TestFriendRequestCarriesTheNoteAndLeavesATrailInTheThread(t *testing.T) {
	line := &socialLine{connected: true}
	conv := kocThread()
	app, ms := socialApp(line, conv, nil)

	code, body := socialCall(app, "POST", "/conversations/"+conv.ID.Hex()+"/zalo/friend-request", `{"message":"  Xin chào, mình là Việt bên nhãn hàng.  "}`)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if len(line.requests) != 1 || line.requests[0] != "42: Xin chào, mình là Việt bên nhãn hàng." {
		t.Fatalf("the note is trimmed and goes to the KOC's Zalo id: %v", line.requests)
	}
	if len(ms.created) != 1 || ms.created[0].Kind != "system" || !strings.Contains(ms.created[0].Text, "Xin chào, mình là Việt") {
		t.Fatalf("the thread records that a request was sent: %+v", ms.created)
	}
}

func TestFriendRequestNoteIsLimitedTo150CharactersNotBytes(t *testing.T) {
	line := &socialLine{connected: true}
	conv := kocThread()
	app, _ := socialApp(line, conv, nil)
	path := "/conversations/" + conv.ID.Hex() + "/zalo/friend-request"

	ok := strings.Repeat("ạ", 150) // 150 characters, 300 bytes: allowed
	if code, body := socialCall(app, "POST", path, `{"message":"`+ok+`"}`); code != 200 {
		t.Fatalf("150 characters must pass: %d %v", code, body)
	}
	if code, _ := socialCall(app, "POST", path, `{"message":"`+ok+`a"}`); code != 400 {
		t.Fatalf("151 characters must be refused, got %d", code)
	}
	if code, _ := socialCall(app, "POST", path, `{"message":"   "}`); code != 400 {
		t.Fatalf("an empty note must be refused, got %d", code)
	}
	if len(line.requests) != 1 {
		t.Fatalf("only the valid one was sent: %v", line.requests)
	}
}

func TestNobodyIsAskedTwiceOrAskedWhenAlreadyAFriend(t *testing.T) {
	for name, st := range map[string]service.ZaloFriend{"friend": {IsFriend: true}, "asked": {RequestSent: true}} {
		line := &socialLine{connected: true, friend: st}
		conv := kocThread()
		app, _ := socialApp(line, conv, nil)
		if code, _ := socialCall(app, "POST", "/conversations/"+conv.ID.Hex()+"/zalo/friend-request", `{"message":"Chào bạn"}`); code != 409 || len(line.requests) != 0 {
			t.Fatalf("%s: want 409 and nothing sent, got %d %v", name, code, line.requests)
		}
	}
}

func TestFriendRequestNeedsTheLineAndReportsWhatZaloAnswered(t *testing.T) {
	conv := kocThread()
	app, _ := socialApp(&socialLine{connected: false}, conv, nil)
	if code, _ := socialCall(app, "POST", "/conversations/"+conv.ID.Hex()+"/zalo/friend-request", `{"message":"Chào"}`); code != 422 {
		t.Fatalf("a line that is down: 422, got %d", code)
	}
	app, _ = socialApp(&socialLine{connected: true, sendErr: errors.New("Zalo từ chối")}, conv, nil)
	code, body := socialCall(app, "POST", "/conversations/"+conv.ID.Hex()+"/zalo/friend-request", `{"message":"Chào"}`)
	if code != 502 || !strings.Contains(body["message"].(string), "Zalo từ chối") {
		t.Fatalf("Zalo's refusal is shown: %d %v", code, body)
	}
}

func TestFriendStateTellsTheScreenWhatToOffer(t *testing.T) {
	conv := kocThread()
	app, _ := socialApp(&socialLine{connected: true, friend: service.ZaloFriend{RequestSent: true}}, conv, nil)
	_, body := socialCall(app, "GET", "/conversations/"+conv.ID.Hex()+"/zalo/friend", "")
	d := body["data"].(map[string]any)
	if d["available"] != true || d["isFriend"] != false || d["requestSent"] != true || d["noteMax"].(float64) != 150 {
		t.Fatalf("%v", d)
	}
	app, _ = socialApp(&socialLine{connected: false}, conv, nil)
	_, body = socialCall(app, "GET", "/conversations/"+conv.ID.Hex()+"/zalo/friend", "")
	if d := body["data"].(map[string]any); d["available"] != false || d["reason"] == "" {
		t.Fatalf("a line that is down says why: %v", d)
	}
}

func inboundWithIDs() *domain.Message {
	return &domain.Message{ID: domain.NewID(), Direction: domain.DirectionInbound, Text: "ok bạn", ZaloMsgID: "100", ZaloCliMsgID: "200"}
}

func TestReactionGoesToZaloWithTheMessageIdsAndIsRemembered(t *testing.T) {
	line := &socialLine{connected: true}
	conv, msg := kocThread(), inboundWithIDs()
	app, ms := socialApp(line, conv, msg)
	path := "/conversations/" + conv.ID.Hex() + "/messages/" + msg.ID.Hex() + "/reaction"

	code, body := socialCall(app, "PUT", path, `{"reaction":"heart"}`)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if len(line.reacted) != 1 || line.reacted[0] != "42/100/200//-heart" {
		t.Fatalf("Zalo gets the KOC id, both message ids and its own code: %v", line.reacted)
	}
	if len(ms.reactions) != 1 || ms.reactions[0] != "heart" {
		t.Fatalf("our reaction is stored by its key: %v", ms.reactions)
	}
	if d := body["data"].(map[string]any); d["reaction"] != "heart" {
		t.Fatalf("the answer carries the reaction: %v", d)
	}
	if _, leaked := body["data"].(map[string]any)["ZaloMsgID"]; leaked {
		t.Fatal("the Zalo ids stay on the server")
	}

	// taking it back sends the empty code
	if code, _ := socialCall(app, "PUT", path, `{"reaction":""}`); code != 200 || line.reacted[1] != "42/100/200/" {
		t.Fatalf("take back: %d %v", code, line.reacted)
	}
}

func TestOnlyKnownReactionsOnMessagesTheKocSentAreAccepted(t *testing.T) {
	line := &socialLine{connected: true}
	conv, msg := kocThread(), inboundWithIDs()
	app, _ := socialApp(line, conv, msg)
	path := "/conversations/" + conv.ID.Hex() + "/messages/" + msg.ID.Hex() + "/reaction"
	if code, _ := socialCall(app, "PUT", path, `{"reaction":"/-heart"}`); code != 400 {
		t.Fatalf("a raw Zalo code is not a key: %d", code)
	}

	for name, m := range map[string]*domain.Message{
		"our own message":       {ID: domain.NewID(), Direction: domain.DirectionOutbound, ZaloMsgID: "1", ZaloCliMsgID: "2"},
		"stored before the ids": {ID: domain.NewID(), Direction: domain.DirectionInbound},
		"a timeline note":       {ID: domain.NewID(), Direction: domain.DirectionInbound, Kind: "system", ZaloMsgID: "1", ZaloCliMsgID: "2"},
	} {
		app, _ := socialApp(line, conv, m)
		if code, _ := socialCall(app, "PUT", "/conversations/"+conv.ID.Hex()+"/messages/"+m.ID.Hex()+"/reaction", `{"reaction":"like"}`); code != 409 {
			t.Fatalf("%s: want 409, got %d", name, code)
		}
	}
	if len(line.reacted) != 0 {
		t.Fatalf("nothing reached Zalo: %v", line.reacted)
	}
}

func TestTheMessageListSaysWhichMessagesCanBeReactedTo(t *testing.T) {
	conv, msg := kocThread(), inboundWithIDs()
	app, _ := socialApp(&socialLine{connected: true}, conv, msg)
	_, body := socialCall(app, "GET", "/conversations/"+conv.ID.Hex()+"/messages", "")
	items := body["data"].([]any)
	if items[0].(map[string]any)["canReact"] != true {
		t.Fatalf("the KOC's message can be reacted to: %v", items[0])
	}
	if _, has := items[1].(map[string]any)["canReact"]; has {
		t.Fatalf("our own message cannot: %v", items[1])
	}
}
