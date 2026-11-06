package repository

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// These conditions used to be written inline in the services. They are copied here word for word: the refactor must
// not change who the sweep chases or which KOCs the dashboard lists.

func TestTaskConditionsAreExactlyWhatTheDashboardAlwaysUsed(t *testing.T) {
	want := map[domain.TaskKind]bson.M{
		domain.TaskNeedCall:         {"callTaskPending": true},
		domain.TaskNeedApproval:     {"stage": domain.StageAwaitingApproval},
		domain.TaskNeedShip:         {"stage": domain.StageAgreed},
		domain.TaskDelayedShipments: {"shippingDelayedAlert": true},
		domain.TaskStalled:          {"$or": []bson.M{{"stage": domain.StageStalled}, {"stalledFlag": true}}},
		domain.TaskPendingReturn:    {"stage": bson.M{"$in": []string{domain.StageNotFit, domain.StageReturning}}},
	}
	if len(domain.AllTaskKinds) != len(want) {
		t.Fatalf("the dashboard has %d task lists, the test knows %d", len(domain.AllTaskKinds), len(want))
	}
	for _, kind := range domain.AllTaskKinds {
		got, ok := taskCondition(kind)
		if !ok || !reflect.DeepEqual(got, want[kind]) {
			t.Fatalf("%s: got %v want %v", kind, got, want[kind])
		}
	}
	if _, ok := taskCondition("nonsense"); ok {
		t.Fatal("an unknown task list must be refused")
	}
}

func TestSweepConditionsAreExactlyWhatTheSweepAlwaysUsed(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		got  bson.M
		want bson.M
	}{
		{"active", activeCondition(), bson.M{"status": domain.KocActive}},
		{"dormant due", dormantDueCondition(now),
			bson.M{"status": domain.KocActive, "stage": domain.StageDormant, "dormantUntil": bson.M{"$ne": nil, "$lte": now}}},
		{"to chase", toChaseCondition(), bson.M{
			"status": domain.KocActive, "daHopTac": false, "dnc": false,
			"stage": bson.M{"$in": bson.A{domain.StageContacted, domain.StageNoReply}},
		}},
		{"in shipping", inShippingCondition(), bson.M{"status": domain.KocActive, "stage": domain.StageShipping}},
		{"delivered", deliveredCondition(), bson.M{"status": domain.KocActive, "dnc": false, "stage": domain.StageDelivered}},
		{"not fit awaiting return", notFitAwaitingReturnCondition(),
			bson.M{"status": domain.KocActive, "dnc": false, "stage": domain.StageNotFit, "returnTrackingCode": ""}},
	}
	for _, c := range cases {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Fatalf("%s: got %v want %v", c.name, c.got, c.want)
		}
	}
}

func TestIdentityConditionMatchesTheWayTheServicesAlwaysDid(t *testing.T) {
	if got, want := identityCondition("emanhthethao", ""), (bson.M{"$or": []bson.M{{"tiktokId": "emanhthethao"}}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("without a phone only the TikTok id counts: %v", got)
	}
	got := identityCondition("emanhthethao", "0901234567")
	want := bson.M{"$or": []bson.M{{"tiktokId": "emanhthethao"}, {"phone": "0901234567"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("with a phone either one matches: %v", got)
	}
}

func TestActiveContactConditionKeepsTheOrderAndSkipsEmptyValues(t *testing.T) {
	got, ok := activeContactCondition("emanhthethao", []string{"0901111111", "0902222222"})
	want := bson.M{"status": domain.KocActive, "$or": []bson.M{{"tiktokId": "emanhthethao"}, {"phone": "0901111111"}, {"phone": "0902222222"}}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	got, ok = activeContactCondition("", []string{"", "0902222222"})
	if !ok || !reflect.DeepEqual(got, bson.M{"status": domain.KocActive, "$or": []bson.M{{"phone": "0902222222"}}}) {
		t.Fatalf("empty values must be skipped: %v", got)
	}
	if _, ok := activeContactCondition("", []string{"", ""}); ok {
		t.Fatal("with nothing to look for no query may be made")
	}
}

func TestEmailAndQualifiedConditionsAreUnchanged(t *testing.T) {
	if got, want := activeByEmailCondition("a@x.vn"), (bson.M{"status": domain.KocActive, "$or": bson.A{bson.M{"email": "a@x.vn"}, bson.M{"contacts.value": "a@x.vn"}}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if got, want := qualifiedNotCollaboratedCondition(), (bson.M{"status": domain.KocActive, "daHopTac": false, "stage": domain.StageQualified}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestLookupConditionsUsedByTheHandlersAreUnchanged(t *testing.T) {
	ids := []domain.ID{domain.NewID(), domain.NewID()}
	if got, want := idsCondition(ids), (bson.M{"_id": bson.M{"$in": ids}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if got, want := notArchivedCondition(), (bson.M{"status": bson.M{"$ne": domain.KocArchived}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if got, want := activeByTiktokIDsCondition([]string{"a", "b"}), (bson.M{"tiktokId": bson.M{"$in": []string{"a", "b"}}, "status": domain.KocActive}); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestKocSearchConditionIsActiveOnlyAndFiltersOnlyWhatIsGiven(t *testing.T) {
	base := kocSearchCondition(domain.KocQuery{})
	if len(base) != 1 || base["status"] != domain.KocActive {
		t.Fatalf("an empty search is just 'active KOCs': %v", base)
	}
	c := kocSearchCondition(domain.KocQuery{Category: "Làm đẹp", Stage: "AGREED", Source: "Zalo", Search: "a.b", SearchID: "a.b"})
	if c["category"] != "Làm đẹp" || c["stage"] != "AGREED" || c["source"] != "Zalo" || c["status"] != domain.KocActive {
		t.Fatalf("%v", c)
	}
	or, ok := c["$or"].([]bson.M)
	if !ok || len(or) != 4 {
		t.Fatalf("a text search looks in name, tiktok id, phone and e-mail: %v", c["$or"])
	}
	re := or[0]["displayName"].(primitive.Regex)
	if re.Pattern != `a\.b` || re.Options != "i" {
		t.Fatalf("the text is escaped and case-insensitive: %+v", re)
	}
}

func TestPatchUpdateWritesOnlyTheGivenFieldsAndRevokesOnRequest(t *testing.T) {
	now := time.Unix(1700000000, 0)
	name := "Tên"
	u := patchUpdate(domain.UserChange{Name: &name}, now)
	set := u["$set"].(bson.M)
	if len(set) != 2 || set["name"] != "Tên" || set["updatedAt"] != now {
		t.Fatalf("%v", u)
	}
	if _, has := u["$inc"]; has {
		t.Fatal("a rename must not revoke sessions")
	}
	role, status := "member", "suspended"
	u = patchUpdate(domain.UserChange{Role: &role, Status: &status, RevokeTokens: true}, now)
	set = u["$set"].(bson.M)
	if set["role"] != "member" || set["status"] != "suspended" || len(set) != 3 {
		t.Fatalf("%v", u)
	}
	if inc, ok := u["$inc"].(bson.M); !ok || inc["tokenVersion"] != 1 {
		t.Fatalf("revoking bumps the token version: %v", u)
	}
}

func TestUserSearchConditionFiltersOnlyWhatIsGiven(t *testing.T) {
	if c := userSearchCondition(domain.UserQuery{}); len(c) != 0 {
		t.Fatalf("an empty query matches everyone: %v", c)
	}
	c := userSearchCondition(domain.UserQuery{Search: "a+b", Role: "admin", Status: "active"})
	if c["role"] != "admin" || c["status"] != "active" {
		t.Fatalf("%v", c)
	}
	or := c["$or"].([]bson.M)
	if re := or[0]["name"].(primitive.Regex); re.Pattern != `a\+b` || len(or) != 2 {
		t.Fatalf("the search looks in name and e-mail, escaped: %v", or)
	}
}

func TestTheAdminConditionsDifferOnlyBySuspension(t *testing.T) {
	if adminsCondition()["role"] != domain.RoleAdmin {
		t.Fatal("admins")
	}
	a := activeAdminsCondition()
	if a["role"] != domain.RoleAdmin || a["status"].(bson.M)["$ne"] != domain.UserSuspended {
		t.Fatalf("%v", a)
	}
}
