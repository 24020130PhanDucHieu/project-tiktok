package repository

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

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
