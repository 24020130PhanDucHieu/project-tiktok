package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// StageStat aggregates KOCs of one pipeline stage.
type StageStat struct {
	Stage    string  `bson:"_id" json:"stage"`
	Count    int64   `bson:"count" json:"count"`
	VideoGmv float64 `bson:"videoGmv" json:"videoGmv"`
	CastFee  float64 `bson:"castFee" json:"castFee"`
}

// StageStats groups active KOCs by stage (served by the {status, stage} index).
func (r *KocRepository) StageStats(ctx context.Context) ([]StageStat, error) {
	cur, err := r.coll.Aggregate(ctx, bson.A{
		bson.M{"$match": bson.M{"status": domain.KocActive}},
		bson.M{"$group": bson.M{"_id": "$stage", "count": bson.M{"$sum": 1},
			"videoGmv": bson.M{"$sum": "$videoGmv"}, "castFee": bson.M{"$sum": "$castFee"}}},
	})
	if err != nil {
		return nil, err
	}
	out := []StageStat{}
	return out, cur.All(ctx, &out)
}

// BatchFunnel is the funnel of one campaign (booking batch), derived purely from timestamps.
type BatchFunnel struct {
	Campaign  string  `bson:"_id" json:"campaign"`
	Pipeline  int64   `bson:"pipeline" json:"pipeline"`
	Contacted int64   `bson:"contacted" json:"contacted"`
	Replied   int64   `bson:"replied" json:"replied"`
	Agreed    int64   `bson:"agreed" json:"agreed"`
	Delivered int64   `bson:"delivered" json:"delivered"`
	Posted    int64   `bson:"posted" json:"posted"`
	Gmv       float64 `bson:"gmv" json:"gmv"`
}

// Funnels aggregates the funnel per campaign.
func (r *KocRepository) Funnels(ctx context.Context) ([]BatchFunnel, error) {
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
	out := []BatchFunnel{}
	return out, cur.All(ctx, &out)
}

// TaskKoc is the slim projection shown in the "today" task list.
type TaskKoc struct {
	ID           string `bson:"_id" json:"id"`
	TiktokID     string `bson:"tiktokId" json:"tiktokId"`
	DisplayName  string `bson:"displayName" json:"displayName"`
	AvatarURL    string `bson:"avatarUrl" json:"avatarUrl"`
	Phone        string `bson:"phone" json:"phone"`
	Stage        string `bson:"stage" json:"stage"`
	TrackingCode string `bson:"trackingCode" json:"trackingCode"`
}

// FindTasks returns a small projected list of KOCs matching a task condition.
func (r *KocRepository) FindTasks(ctx context.Context, cond bson.M, limit int64) ([]TaskKoc, error) {
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
	out := make([]TaskKoc, len(kocs))
	for i, k := range kocs {
		out[i] = TaskKoc{ID: k.ID.Hex(), TiktokID: k.TiktokID, DisplayName: k.DisplayName, AvatarURL: k.AvatarURL,
			Phone: k.Phone, Stage: k.Stage, TrackingCode: k.TrackingCode}
	}
	return out, nil
}
