package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// StageStats groups active KOCs by stage (served by the {status, stage} index).
func (r *KocRepository) StageStats(ctx context.Context) ([]domain.StageStat, error) {
	cur, err := r.coll.Aggregate(ctx, bson.A{
		bson.M{"$match": bson.M{"status": domain.KocActive}},
		bson.M{"$group": bson.M{"_id": "$stage", "count": bson.M{"$sum": 1},
			"videoGmv": bson.M{"$sum": "$videoGmv"}, "castFee": bson.M{"$sum": "$castFee"}}},
	})
	if err != nil {
		return nil, err
	}
	out := []domain.StageStat{}
	return out, cur.All(ctx, &out)
}

// Funnels aggregates the funnel per campaign.
func (r *KocRepository) Funnels(ctx context.Context) ([]domain.BatchFunnel, error) {
	has := func(field string) bson.M {
		return bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{"$" + field, nil}}, 1, 0}}}
	}
	cur, err := r.coll.Aggregate(ctx, bson.A{
		bson.M{"$match": bson.M{"status": domain.KocActive}},
		bson.M{"$group": bson.M{
			"_id": "$campaign", "pipeline": bson.M{"$sum": 1},
			"contacted": has("invitedAt"), "replied": has("lastRepliedAt"),
			"agreed": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$or": bson.A{
				bson.M{"$ne": bson.A{"$shippedAt", nil}},
				bson.M{"$in": bson.A{"$stage", bson.A{domain.StageAgreed, domain.StageShipping, domain.StageDelivered,
					domain.StageVideoPosted, domain.StageCompleted, domain.StageNotFit, domain.StageReturning,
					domain.StageReturned, domain.StageStalled}}},
			}}, 1, 0}}},
			"delivered": has("deliveredAt"), "posted": has("videoPostedAt"),
			"gmv": bson.M{"$sum": "$videoGmv"},
		}},
		bson.M{"$sort": bson.M{"pipeline": -1}},
		bson.M{"$limit": 20},
	})
	if err != nil {
		return nil, err
	}
	out := []domain.BatchFunnel{}
	return out, cur.All(ctx, &out)
}

// FindTasks returns a small projected list of KOCs matching a task condition.
func (r *KocRepository) FindTasks(ctx context.Context, kind domain.TaskKind, limit int64) ([]domain.TaskKoc, error) {
	cond, ok := taskCondition(kind)
	if !ok {
		return nil, fmt.Errorf("unknown task list %q", kind)
	}
	filter := bson.M{"status": domain.KocActive}
	for k, v := range cond {
		filter[k] = v
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().SetLimit(limit).SetSort(bson.D{{Key: "updatedAt", Value: 1}}).
		SetProjection(bson.M{"tiktokId": 1, "displayName": 1, "avatarUrl": 1, "phone": 1, "stage": 1, "trackingCode": 1}))
	if err != nil {
		return nil, err
	}
	var kocs []domain.Koc
	if err := cur.All(ctx, &kocs); err != nil {
		return nil, err
	}
	out := make([]domain.TaskKoc, len(kocs))
	for i, k := range kocs {
		out[i] = domain.TaskKoc{ID: k.ID.Hex(), TiktokID: k.TiktokID, DisplayName: k.DisplayName, AvatarURL: k.AvatarURL,
			Phone: k.Phone, Stage: k.Stage, TrackingCode: k.TrackingCode}
	}
	return out, nil
}
