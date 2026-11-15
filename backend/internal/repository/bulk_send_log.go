package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// BulkSendLogRepository keeps the outcome of every KOC of every bulk send ("bulk_send_log").
type BulkSendLogRepository struct{ coll *mongo.Collection }

// NewBulkSendLogRepository builds the repository.
func NewBulkSendLogRepository(db *mongo.Database) *BulkSendLogRepository {
	return &BulkSendLogRepository{coll: db.Collection("bulk_send_log")}
}

// Record stores the rows (their At is set here when empty).
func (r *BulkSendLogRepository) Record(ctx context.Context, rows []domain.BulkSendLog) error {
	if len(rows) == 0 {
		return nil
	}
	docs := make([]any, len(rows))
	for i := range rows {
		if rows[i].At.IsZero() {
			rows[i].At = time.Now()
		}
		docs[i] = rows[i]
	}
	_, err := r.coll.InsertMany(ctx, docs)
	return err
}

// Recent returns the newest rows first (at most limit).
func (r *BulkSendLogRepository) Recent(ctx context.Context, limit int64) ([]domain.BulkSendLog, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	out := []domain.BulkSendLog{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
