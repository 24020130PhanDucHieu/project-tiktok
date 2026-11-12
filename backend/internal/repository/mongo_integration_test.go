package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/tiktoday/kocflow-api/internal/database"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// These tests need a real MongoDB and are skipped without one:
//
//	docker run -d -p 27099:27017 mongo:8
//	MONGO_TEST_URI=mongodb://127.0.0.1:27099 go test ./internal/repository
func testDB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("MONGO_TEST_URI is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, db, err := database.Connect(ctx, uri, fmt.Sprintf("kocflow_test_%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	if err := database.EnsureIndexes(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func isConflict(err error) bool {
	var f *domain.Fault
	return errors.As(err, &f) && f.Kind == domain.FaultConflict
}

func newKoc(id string) *domain.Koc {
	k := domain.NewKoc()
	k.TiktokID, k.DisplayName, k.Status, k.Stage = id, id, domain.KocActive, domain.StageContacted
	return k
}

// Two writers that read the same KOC used to overwrite each other: the second Save replaced the whole document and
// silently undid the first one (the sweep against the chatbot, or against somebody editing the KOC).
func TestASecondSaveFromAStaleReadIsRefusedInsteadOfOverwriting(t *testing.T) {
	db := testDB(t)
	repo := repository.NewKocRepository(db)
	ctx := context.Background()
	k := newKoc("linh.review")
	if err := repo.Create(ctx, k); err != nil {
		t.Fatal(err)
	}

	sweep, _ := repo.FindActiveByID(ctx, k.ID.Hex())
	chatbot, _ := repo.FindActiveByID(ctx, k.ID.Hex())

	chatbot.Stage, chatbot.ShippingAddress = domain.StageAgreed, "12 đường Láng"
	if err := repo.Save(ctx, chatbot); err != nil {
		t.Fatalf("the first writer saves: %v", err)
	}
	sweep.ReminderStep = 2 // decided on the old picture of the KOC
	if err := repo.Save(ctx, sweep); !isConflict(err) {
		t.Fatalf("the stale writer must be told about the conflict, got %v", err)
	}

	got, _ := repo.FindActiveByID(ctx, k.ID.Hex())
	if got.Stage != domain.StageAgreed || got.ShippingAddress != "12 đường Láng" || got.ReminderStep != 0 {
		t.Fatalf("the first writer's change must survive untouched: %+v", got)
	}
	// after a fresh read the same change goes through, and the same holder can save again and again
	got.ReminderStep = 2
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("a save from a fresh read: %v", err)
	}
	got.ReminderStep = 3
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("the holder of the newest version keeps saving: %v", err)
	}
}

func TestAKocStoredBeforeVersionsExistedCanStillBeSaved(t *testing.T) {
	db := testDB(t)
	repo := repository.NewKocRepository(db)
	ctx := context.Background()
	k := newKoc("old.record")
	if err := repo.Create(ctx, k); err != nil {
		t.Fatal(err)
	}
	// what every document written by the previous release looks like: no version field at all
	if _, err := db.Collection("kocs").UpdateByID(ctx, k.ID, bson.M{"$unset": bson.M{"version": ""}}); err != nil {
		t.Fatal(err)
	}
	old, _ := repo.FindActiveByID(ctx, k.ID.Hex())
	old.Stage = domain.StageReplied
	if err := repo.Save(ctx, old); err != nil {
		t.Fatalf("an old document must save: %v", err)
	}
	if got, _ := repo.FindActiveByID(ctx, k.ID.Hex()); got.Stage != domain.StageReplied {
		t.Fatalf("stage %s", got.Stage)
	}
}

// winningPlan returns the query plan MongoDB chose, as text, to look for IXSCAN / COLLSCAN in it.
func winningPlan(t *testing.T, db *mongo.Database, coll string, filter bson.M, sort bson.D) string {
	t.Helper()
	find := bson.D{{Key: "find", Value: coll}, {Key: "filter", Value: filter}}
	if sort != nil {
		find = append(find, bson.E{Key: "sort", Value: sort})
	}
	var out bson.M
	if err := db.RunCommand(context.Background(), bson.D{{Key: "explain", Value: find}, {Key: "verbosity", Value: "queryPlanner"}}).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(out["queryPlanner"].(bson.M)["winningPlan"])
}

func TestTheFrequentLookupsUseAnIndex(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	// a few documents so the planner has collections to plan against
	_, _ = db.Collection("messages").InsertOne(ctx, bson.M{"externalId": "gmail:1", "sentAt": time.Now()})
	_, _ = db.Collection("collabs").InsertOne(ctx, bson.M{"kocId": domain.NewID(), "seq": 1, "startedAt": time.Now()})
	_, _ = db.Collection("kocs").InsertOne(ctx, bson.M{"tiktokId": "a", "email": "a@x.vn", "status": domain.KocActive})

	for _, c := range []struct {
		name, coll string
		filter     bson.M
		sort       bson.D
	}{
		{"gmail de-duplication", "messages", bson.M{"externalId": "gmail:1"}, nil},
		{"collaborations of a KOC", "collabs", bson.M{"kocId": domain.NewID()}, bson.D{{Key: "seq", Value: -1}}},
		{"collaboration overview", "collabs", bson.M{}, bson.D{{Key: "startedAt", Value: -1}}},
		{"KOC by email or contact", "kocs", bson.M{"status": domain.KocActive, "$or": bson.A{bson.M{"email": "a@x.vn"}, bson.M{"contacts.value": "a@x.vn"}}}, nil},
	} {
		plan := winningPlan(t, db, c.coll, c.filter, c.sort)
		if strings.Contains(plan, "COLLSCAN") || !strings.Contains(plan, "IXSCAN") {
			t.Errorf("%s scans the whole collection: %s", c.name, plan)
		}
	}
}

func TestEnsureIndexesCanRunOnEveryStart(t *testing.T) {
	db := testDB(t) // already ran once
	if err := database.EnsureIndexes(context.Background(), db); err != nil {
		t.Fatalf("a second start must not fail on the existing indexes: %v", err)
	}
}

func TestTheBriefListCarriesOnlyWhatThePageNeeds(t *testing.T) {
	db := testDB(t)
	repo := repository.NewKocRepository(db)
	ctx := context.Background()
	active := newKoc("linh.review")
	active.AvatarURL, active.Phone = "https://cdn/a.jpg", "0327637213"
	archived := newKoc("gone")
	archived.Status = domain.KocArchived
	for _, k := range []*domain.Koc{active, archived} {
		if err := repo.Create(ctx, k); err != nil {
			t.Fatal(err)
		}
	}
	briefs, err := repo.ListBriefsNotArchived(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 1 || briefs[0].ID != active.ID || briefs[0].TiktokID != "linh.review" || briefs[0].AvatarURL != "https://cdn/a.jpg" {
		t.Fatalf("only the KOC that is not archived, with its id, handle, name and avatar: %+v", briefs)
	}
}

// A thread that a KOC's message has just made unread must stay unread when somebody else updates one of its fields.
func TestTheFieldUpdatesLeaveTheOtherFieldsAlone(t *testing.T) {
	db := testDB(t)
	repo := repository.NewConversationRepository(db)
	ctx := context.Background()
	c := &domain.Conversation{Channel: domain.ChannelInternal, ContactName: "Linh", ContactHandle: "@linh", ContactPhone: "0327637213", Status: domain.ConversationOpen}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	held, _ := repo.FindByID(ctx, c.ID.Hex()) // what a slow request read earlier
	// meanwhile a KOC's message arrives
	if _, err := repo.UpsertZaloInbound(ctx, "u1", "Zalo Linh", "alo", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("conversations").UpdateByID(ctx, c.ID, bson.M{"$set": bson.M{"unreadCount": 3}}); err != nil {
		t.Fatal(err)
	}

	if err := repo.SetLastMessageAt(ctx, held.ID, "Chào Linh", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetZaloContact(ctx, held.ID, domain.ZaloContact{Channel: domain.ChannelZalo, ExternalContactID: "42", Phone: "0988111222"}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByID(ctx, c.ID.Hex())
	if got.UnreadCount != 3 {
		t.Fatalf("the unread count raised meanwhile must survive, got %d", got.UnreadCount)
	}
	if got.LastMessage != "Chào Linh" || got.ExternalContactID != "42" || got.Channel != domain.ChannelZalo || got.ContactPhone != "0988111222" {
		t.Fatalf("the named fields are written: %+v", got)
	}
	if got.ContactName != "Linh" {
		t.Fatalf("an empty name leaves the stored one alone, got %q", got.ContactName)
	}
	if err := repo.ResetThread(ctx, held.ID, "Đã làm mới", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.FindByID(ctx, c.ID.Hex()); got.UnreadCount != 0 || got.LastMessage != "Đã làm mới" {
		t.Fatalf("a cleared thread is read again: %+v", got)
	}
}

// The link and the Shop Ads commission of an open-collaboration product are kept per product: saved once, read back
// for every later KOC, and a second save replaces them instead of adding a second record.
func TestWhatIsSavedForAnOpenProductIsKeptPerProduct(t *testing.T) {
	db := testDB(t)
	repo := repository.NewOpenProductRepository(db)
	ctx := context.Background()

	if got, err := repo.Get(ctx, "1736613031755089792"); err != nil || got != nil {
		t.Fatalf("nothing saved yet: %+v %v", got, err)
	}
	if err := repo.Upsert(ctx, &domain.OpenProduct{ProductID: "1736613031755089792", Link: "https://vt.tiktok.com/ZS1/", AdsCommissionRate: 12.5, UpdatedBy: "a@shop.vn"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, &domain.OpenProduct{ProductID: "1736613031755089792", Link: "https://vt.tiktok.com/ZS2/", AdsCommissionRate: 15, UpdatedBy: "b@shop.vn"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "1736613031755089792")
	if err != nil || got == nil || got.Link != "https://vt.tiktok.com/ZS2/" || got.AdsCommissionRate != 15 || got.UpdatedBy != "b@shop.vn" || got.UpdatedAt.IsZero() {
		t.Fatalf("the latest save is what is read back: %+v %v", got, err)
	}
	if n, _ := db.Collection("open_products").CountDocuments(ctx, bson.M{}); n != 1 {
		t.Fatalf("one record per product, got %d", n)
	}
	if other, _ := repo.Get(ctx, "999"); other != nil {
		t.Fatalf("another product has nothing: %+v", other)
	}
}

// The system must not message creators by itself until an admin turned the switch on: a database that has never heard of
// the switch means off, and who changed it last is remembered.
func TestTheAutomationSwitchIsOffUntilSetAndRemembersWho(t *testing.T) {
	db := testDB(t)
	repo := repository.NewAutomationRepository(db)
	ctx := context.Background()

	if on, err := repo.Enabled(ctx); err != nil || on {
		t.Fatalf("never set means off: %v %v", on, err)
	}
	if err := repo.SetEnabled(ctx, true, "admin@shop.vn"); err != nil {
		t.Fatal(err)
	}
	st, err := repo.State(ctx)
	if err != nil || !st.Enabled || st.UpdatedBy != "admin@shop.vn" || st.UpdatedAt.IsZero() {
		t.Fatalf("on, by whom, when: %+v %v", st, err)
	}
	if err := repo.SetEnabled(ctx, false, "boss@shop.vn"); err != nil {
		t.Fatal(err)
	}
	if on, _ := repo.Enabled(ctx); on {
		t.Fatal("off again")
	}
	if n, _ := db.Collection("app_settings").CountDocuments(ctx, bson.M{"_id": "automation_sending"}); n != 1 {
		t.Fatalf("one record, got %d", n)
	}
}

// The Zalo ids of a message never reach the browser (json "-") but must survive the round trip to the database, and a
// reaction we put on it is stored by key and can be taken back without touching the rest of the message.
func TestAMessageKeepsItsZaloIdsAndTheReactionPutOnIt(t *testing.T) {
	db := testDB(t)
	repo := repository.NewMessageRepository(db)
	ctx := context.Background()
	conv := domain.NewID()
	m := &domain.Message{ConversationID: conv, Direction: domain.DirectionInbound, Text: "ok bạn", Status: domain.MessageDelivered, ZaloMsgID: "100", ZaloCliMsgID: "200"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetReaction(ctx, conv, m.ID.Hex(), "heart"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(ctx, conv, m.ID.Hex())
	if err != nil || got == nil || got.Reaction != "heart" || got.ZaloMsgID != "100" || got.ZaloCliMsgID != "200" || got.Text != "ok bạn" {
		t.Fatalf("reaction and ids: %+v %v", got, err)
	}
	if err := repo.SetReaction(ctx, conv, m.ID.Hex(), ""); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.FindByID(ctx, conv, m.ID.Hex()); got.Reaction != "" || got.ZaloMsgID != "100" {
		t.Fatalf("taking it back keeps the rest: %+v", got)
	}
	// a message of another conversation is not touched
	if err := repo.SetReaction(ctx, domain.NewID(), m.ID.Hex(), "like"); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.FindByID(ctx, conv, m.ID.Hex()); got.Reaction != "" {
		t.Fatalf("wrong conversation must not change it: %+v", got)
	}
}
