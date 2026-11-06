package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// The conditions below are plain functions so they can be tested without a database. Their meaning is named after the
// business question ("who is due to be chased?"), which is what the services ask; how it is written in MongoDB stays here.

// taskCondition is the condition of one dashboard task list (the "status: active" part is added by FindTasks).
func taskCondition(kind domain.TaskKind) (bson.M, bool) {
	switch kind {
	case domain.TaskNeedCall:
		return bson.M{"callTaskPending": true}, true
	case domain.TaskNeedApproval:
		return bson.M{"stage": domain.StageAwaitingApproval}, true
	case domain.TaskNeedShip:
		return bson.M{"stage": domain.StageAgreed}, true
	case domain.TaskDelayedShipments:
		return bson.M{"shippingDelayedAlert": true}, true
	case domain.TaskStalled:
		return bson.M{"$or": []bson.M{{"stage": domain.StageStalled}, {"stalledFlag": true}}}, true
	case domain.TaskPendingReturn:
		return bson.M{"stage": bson.M{"$in": []string{domain.StageNotFit, domain.StageReturning}}}, true
	}
	return nil, false
}

func activeCondition() bson.M { return bson.M{"status": domain.KocActive} }

func dormantDueCondition(now time.Time) bson.M {
	return bson.M{"status": domain.KocActive, "stage": domain.StageDormant, "dormantUntil": bson.M{"$ne": nil, "$lte": now}}
}

func toChaseCondition() bson.M {
	return bson.M{
		"status": domain.KocActive, "daHopTac": false, "dnc": false,
		"stage": bson.M{"$in": bson.A{domain.StageContacted, domain.StageNoReply}},
	}
}

func inShippingCondition() bson.M {
	return bson.M{"status": domain.KocActive, "stage": domain.StageShipping}
}

func deliveredCondition() bson.M {
	return bson.M{"status": domain.KocActive, "dnc": false, "stage": domain.StageDelivered}
}

func notFitAwaitingReturnCondition() bson.M {
	return bson.M{"status": domain.KocActive, "dnc": false, "stage": domain.StageNotFit, "returnTrackingCode": ""}
}

// FindActive returns every active KOC.
func (r *KocRepository) FindActive(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, activeCondition())
}

// FindDormantDue returns dormant KOCs whose waiting period has ended.
func (r *KocRepository) FindDormantDue(ctx context.Context, now time.Time) ([]*domain.Koc, error) {
	return r.Find(ctx, dormantDueCondition(now))
}

// FindToChase returns contacted KOCs that have not replied, who may still be reminded.
func (r *KocRepository) FindToChase(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, toChaseCondition())
}

// FindInShipping returns KOCs whose sample is on its way.
func (r *KocRepository) FindInShipping(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, inShippingCondition())
}

// FindDelivered returns KOCs who received the sample and have not posted yet.
func (r *KocRepository) FindDelivered(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, deliveredCondition())
}

// FindNotFitAwaitingReturn returns KOCs who found the product unsuitable and have not sent the sample back yet.
func (r *KocRepository) FindNotFitAwaitingReturn(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, notFitAwaitingReturnCondition())
}
