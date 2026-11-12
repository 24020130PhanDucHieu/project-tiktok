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

// openProductListLimit caps the list: the team keeps a few dozen products, never thousands.
const openProductListLimit = 500

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

// Upsert saves the link and the ads commission of a product, creating the record the first time. The name and the
// picture are only written when given, so saving the link from a collaboration does not erase what the import read.
func (r *OpenProductRepository) Upsert(ctx context.Context, p *domain.OpenProduct) error {
	p.UpdatedAt = time.Now()
	set := bson.M{"link": p.Link, "adsCommissionRate": p.AdsCommissionRate, "updatedBy": p.UpdatedBy, "updatedAt": p.UpdatedAt}
	if p.Name != "" {
		set["name"] = p.Name
	}
	if p.Image != "" {
		set["image"] = p.Image
	}
	_, err := r.coll.UpdateByID(ctx, p.ProductID, bson.M{"$set": set}, options.Update().SetUpsert(true))
	return err
}

// List returns every saved product, the most recently changed first.
func (r *OpenProductRepository) List(ctx context.Context) ([]domain.OpenProduct, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(openProductListLimit))
	if err != nil {
		return nil, err
	}
	out := []domain.OpenProduct{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetAdsRate changes only the Shop Ads commission of a saved product. It reports false when the product is not saved.
func (r *OpenProductRepository) SetAdsRate(ctx context.Context, productID string, rate float64, by string) (bool, error) {
	res, err := r.coll.UpdateByID(ctx, productID, bson.M{"$set": bson.M{"adsCommissionRate": rate, "updatedBy": by, "updatedAt": time.Now()}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// Delete forgets a product's link. It reports false when nothing was saved for it.
func (r *OpenProductRepository) Delete(ctx context.Context, productID string) (bool, error) {
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": productID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}
