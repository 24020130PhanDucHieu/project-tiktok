package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

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

// identityCondition matches a KOC by TikTok id, or by phone number when one is given.
func identityCondition(tiktokID, phone string) bson.M {
	or := []bson.M{{"tiktokId": tiktokID}}
	if phone != "" {
		or = append(or, bson.M{"phone": phone})
	}
	return bson.M{"$or": or}
}

// activeContactCondition matches an active KOC by any of the identifiers a chat gives us: the TikTok id of the
// conversation and the phone numbers seen so far. ok is false when there is nothing to look for.
func activeContactCondition(tiktokID string, phones []string) (bson.M, bool) {
	var or []bson.M
	if tiktokID != "" {
		or = append(or, bson.M{"tiktokId": tiktokID})
	}
	for _, p := range phones {
		if p != "" {
			or = append(or, bson.M{"phone": p})
		}
	}
	if len(or) == 0 {
		return nil, false
	}
	return bson.M{"status": domain.KocActive, "$or": or}, true
}

func activeByEmailCondition(email string) bson.M {
	return bson.M{"status": domain.KocActive, "$or": bson.A{bson.M{"email": email}, bson.M{"contacts.value": email}}}
}

func qualifiedNotCollaboratedCondition() bson.M {
	return bson.M{"status": domain.KocActive, "daHopTac": false, "stage": domain.StageQualified}
}

// FindByIdentity returns the KOC with this TikTok id or phone number (active or not), or (nil, nil).
func (r *KocRepository) FindByIdentity(ctx context.Context, tiktokID, phone string) (*domain.Koc, error) {
	return r.FindOne(ctx, identityCondition(tiktokID, phone))
}

// FindActiveByContact returns the active KOC that owns the conversation's TikTok id or one of the phone numbers.
// With nothing to look for it asks the database nothing and returns (nil, nil).
func (r *KocRepository) FindActiveByContact(ctx context.Context, tiktokID string, phones ...string) (*domain.Koc, error) {
	cond, ok := activeContactCondition(tiktokID, phones)
	if !ok {
		return nil, nil
	}
	return r.FindOne(ctx, cond)
}

// FindActiveByEmail returns the active KOCs that use this e-mail address (main address or any saved contact).
func (r *KocRepository) FindActiveByEmail(ctx context.Context, email string) ([]*domain.Koc, error) {
	return r.Find(ctx, activeByEmailCondition(email))
}

// FindQualifiedNotCollaborated returns the qualified KOCs that never worked with the shop.
func (r *KocRepository) FindQualifiedNotCollaborated(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, qualifiedNotCollaboratedCondition())
}

// FindRecentActive returns the newest active KOCs.
func (r *KocRepository) FindRecentActive(ctx context.Context, limit int64) ([]domain.Koc, error) {
	return r.FindRecent(ctx, activeCondition(), limit)
}

func idsCondition(ids []primitive.ObjectID) bson.M { return bson.M{"_id": bson.M{"$in": ids}} }

func notArchivedCondition() bson.M { return bson.M{"status": bson.M{"$ne": domain.KocArchived}} }

func activeByTiktokIDsCondition(ids []string) bson.M {
	return bson.M{"tiktokId": bson.M{"$in": ids}, "status": domain.KocActive}
}

// FindByIDs returns the KOCs with these ids (active or not).
func (r *KocRepository) FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]*domain.Koc, error) {
	return r.Find(ctx, idsCondition(ids))
}

// FindNotArchived returns every KOC that has not been archived.
func (r *KocRepository) FindNotArchived(ctx context.Context) ([]*domain.Koc, error) {
	return r.Find(ctx, notArchivedCondition())
}

// FindActiveByTiktokIDs returns the active KOCs with these TikTok ids.
func (r *KocRepository) FindActiveByTiktokIDs(ctx context.Context, ids []string) ([]*domain.Koc, error) {
	return r.Find(ctx, activeByTiktokIDsCondition(ids))
}
