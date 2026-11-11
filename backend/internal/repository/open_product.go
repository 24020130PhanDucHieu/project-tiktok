package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// OpenProductRepository persists OpenProduct documents on "open_products", keyed by the TikTok Shop product id.
type OpenProductRepository struct{ coll *mongo.Collection }

// NewOpenProductRepository builds the repository.
func NewOpenProductRepository(db *mongo.Database) *OpenProductRepository {
	return &OpenProductRepository{coll: db.Collection("open_products")}
}

// Get returns what is saved for a product, or nil when nothing is.
func (r *OpenProductRepository) Get(ctx context.Context, productID string) (*domain.OpenProduct, error) {
	var p domain.OpenProduct
	err := r.coll.FindOne(ctx, bson.M{"_id": productID}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Upsert saves the link and the ads commission of a product, creating the record the first time.
func (r *OpenProductRepository) Upsert(ctx context.Context, p *domain.OpenProduct) error {
	p.UpdatedAt = time.Now()
	_, err := r.coll.UpdateByID(ctx, p.ProductID, bson.M{"$set": bson.M{
		"link": p.Link, "adsCommissionRate": p.AdsCommissionRate, "updatedBy": p.UpdatedBy, "updatedAt": p.UpdatedAt,
	}}, options.Update().SetUpsert(true))
	return err
}
