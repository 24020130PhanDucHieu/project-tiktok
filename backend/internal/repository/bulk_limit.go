package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

const bulkLimitID = "bulk_daily_limit"

// DefaultBulkLimit is how many KOCs a day may be started when nobody has chosen.
const DefaultBulkLimit = 30

// BulkLimitRepository keeps how many KOCs a bulk run may start per day.
type BulkLimitRepository struct{ settings *mongo.Collection }

// NewBulkLimitRepository builds the repository on "app_settings".
func NewBulkLimitRepository(db *mongo.Database) *BulkLimitRepository {
	return &BulkLimitRepository{settings: db.Collection("app_settings")}
}

// ValidBulkLimit reports whether n is one of the offered limits.
func ValidBulkLimit(n int) bool { return n == 10 || n == 20 || n == 30 || n == 50 }

// Get returns the limit: 30 when none was set or the stored value is not an offered one.
func (r *BulkLimitRepository) Get(ctx context.Context) int {
	var doc struct {
		Value int `bson:"value"`
	}
	err := r.settings.FindOne(ctx, bson.M{"_id": bulkLimitID}).Decode(&doc)
	if err != nil || !ValidBulkLimit(doc.Value) {
		return DefaultBulkLimit
	}
	return doc.Value
}

// Set stores the limit: only 10, 20, 30 or 50.
func (r *BulkLimitRepository) Set(ctx context.Context, n int) error {
	if !ValidBulkLimit(n) {
		return domain.Invalid("Hạn mức chỉ có thể là 10, 20, 30 hoặc 50 KOC mỗi ngày.")
	}
	_, err := r.settings.UpdateOne(ctx, bson.M{"_id": bulkLimitID}, bson.M{"$set": bson.M{"value": n}}, options.Update().SetUpsert(true))
	return err
}
