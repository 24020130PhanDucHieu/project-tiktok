package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ZaloAccountRepository stores the personal Zalo accounts in the "zalo_accounts" collection.
type ZaloAccountRepository struct{ coll *mongo.Collection }

// NewZaloAccountRepository builds a ZaloAccountRepository.
func NewZaloAccountRepository(db *mongo.Database) *ZaloAccountRepository {
	return &ZaloAccountRepository{coll: db.Collection("zalo_accounts")}
}

// Create stores a new account and sets its id.
func (r *ZaloAccountRepository) Create(ctx context.Context, a *domain.ZaloAccount) error {
	if a.ID.IsZero() {
		a.ID = domain.NewID()
	}
	_, err := r.coll.InsertOne(ctx, a)
	return err
}

// Update replaces the stored account with the same id.
func (r *ZaloAccountRepository) Update(ctx context.Context, a *domain.ZaloAccount) error {
	res, err := r.coll.ReplaceOne(ctx, bson.M{"_id": a.ID}, a)
	if err == nil && res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return err
}

// Find returns one account, or (nil, nil) when there is none.
func (r *ZaloAccountRepository) Find(ctx context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

// FindByUID returns the account of a Zalo user id, or (nil, nil).
func (r *ZaloAccountRepository) FindByUID(ctx context.Context, uid string) (*domain.ZaloAccount, error) {
	return r.findOne(ctx, bson.M{"zaloUid": uid})
}

func (r *ZaloAccountRepository) findOne(ctx context.Context, filter bson.M) (*domain.ZaloAccount, error) {
	var a domain.ZaloAccount
	err := r.coll.FindOne(ctx, filter).Decode(&a)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// List returns every account, oldest first.
func (r *ZaloAccountRepository) List(ctx context.Context) ([]domain.ZaloAccount, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := []domain.ZaloAccount{}
	return out, cur.All(ctx, &out)
}

// Delete removes an account.
func (r *ZaloAccountRepository) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
