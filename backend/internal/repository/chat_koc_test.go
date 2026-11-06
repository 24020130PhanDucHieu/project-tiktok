package repository

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// The documents below are what the koc and outreach services used to build inline. The refactor must write exactly
// the same fields in exactly the same places (always, or only when the conversation is created).

var testContact = domain.KocContact{Handle: "@emanhthethao", Name: "Em Anh", Phone: "0901234567", LastMessage: "Đã thêm KOC vào danh bạ"}

func TestAddingAKocRefreshesTheWholeConversation(t *testing.T) {
	set, onInsert := kocContactUpsert(testContact)
	wantSet := bson.M{"channel": domain.ChannelInternal, "contactName": "Em Anh", "contactHandle": "@emanhthethao",
		"contactPhone": "0901234567", "status": domain.ConversationOpen, "lastMessage": "Đã thêm KOC vào danh bạ"}
	if !reflect.DeepEqual(set, wantSet) || !reflect.DeepEqual(onInsert, bson.M{}) {
		t.Fatalf("set %v onInsert %v", set, onInsert)
	}
}

func TestBulkImportOnlyCreatesAMissingConversationAndNeverTouchesAnExistingOne(t *testing.T) {
	set, onInsert := kocContactEnsure(testContact)
	wantInsert := bson.M{"channel": domain.ChannelInternal, "contactName": "Em Anh", "contactHandle": "@emanhthethao",
		"contactPhone": "0901234567", "status": domain.ConversationOpen, "lastMessage": "Đã thêm KOC vào danh bạ"}
	if len(set) != 0 || !reflect.DeepEqual(onInsert, wantInsert) {
		t.Fatalf("set %v onInsert %v", set, onInsert)
	}
}

func TestTheLastMessageToAKocKeepsTheChannelForCreationOnly(t *testing.T) {
	at := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	set, onInsert := kocMessageUpsert(testContact, at)
	wantSet := bson.M{"contactName": "Em Anh", "contactHandle": "@emanhthethao", "contactPhone": "0901234567",
		"status": domain.ConversationOpen, "lastMessage": "Đã thêm KOC vào danh bạ", "lastMessageAt": at}
	if !reflect.DeepEqual(set, wantSet) || !reflect.DeepEqual(onInsert, bson.M{"channel": domain.ChannelInternal}) {
		t.Fatalf("set %v onInsert %v", set, onInsert)
	}
	if _, ok := set["channel"]; ok {
		t.Fatal("an existing conversation must keep its channel")
	}
}
