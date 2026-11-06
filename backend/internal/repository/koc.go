// Package repository wraps MongoDB collections.
package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// KocRepository persists Koc documents.
type KocRepository struct{ coll *mongo.Collection }

// NewKocRepository builds the repository on the "kocs" collection.
func NewKocRepository(db *mongo.Database) *KocRepository {
	return &KocRepository{coll: db.Collection("kocs")}
}

// List returns a page of KOCs (newest first) and the total match count.
func (r *KocRepository) List(ctx context.Context, filter bson.M, skip, limit int64) ([]domain.Koc, int64, error) {
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetSkip(skip).SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	items := []domain.Koc{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, 0, err
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	return items, total, err
}

// Find returns every KOC matching the filter.
func (r *KocRepository) Find(ctx context.Context, filter bson.M) ([]*domain.Koc, error) {
	cur, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var items []*domain.Koc
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindRecent returns the newest KOCs matching the filter.
func (r *KocRepository) FindRecent(ctx context.Context, filter bson.M, limit int64) ([]domain.Koc, error) {
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	items := []domain.Koc{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindOne returns the first KOC matching the filter, or (nil, nil).
func (r *KocRepository) FindOne(ctx context.Context, filter bson.M) (*domain.Koc, error) {
	var koc domain.Koc
	err := r.coll.FindOne(ctx, filter).Decode(&koc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &koc, err
}

// FindActiveByID returns an active KOC by hex id, or (nil, nil).
func (r *KocRepository) FindActiveByID(ctx context.Context, id string) (*domain.Koc, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, nil
	}
	return r.FindOne(ctx, bson.M{"_id": oid, "status": domain.KocActive})
}

// Create inserts a new KOC.
func (r *KocRepository) Create(ctx context.Context, koc *domain.Koc) error {
	now := time.Now()
	koc.CreatedAt, koc.UpdatedAt = now, now
	if koc.Videos == nil {
		koc.Videos = []domain.VideoItem{}
	}
	res, err := r.coll.InsertOne(ctx, koc)
	if err != nil {
		return err
	}
	koc.ID = domain.ID(res.InsertedID.(primitive.ObjectID))
	return nil
}

// InsertMany inserts many KOCs (used by the seeder).
func (r *KocRepository) InsertMany(ctx context.Context, kocs []*domain.Koc) error {
	now := time.Now()
	docs := make([]any, len(kocs))
	for i, k := range kocs {
		k.CreatedAt, k.UpdatedAt = now, now
		docs[i] = k
	}
	_, err := r.coll.InsertMany(ctx, docs)
	return err
}

// Save replaces the stored document (equivalent of mongoose doc.save()).
func (r *KocRepository) Save(ctx context.Context, koc *domain.Koc) error {
	koc.UpdatedAt = time.Now()
	if koc.Videos == nil {
		koc.Videos = []domain.VideoItem{}
	}
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": koc.ID}, koc)
	return err
}

// Archive marks an active KOC archived and returns it.
func (r *KocRepository) Archive(ctx context.Context, id string) (*domain.Koc, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, nil
	}
	var koc domain.Koc
	err = r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": oid, "status": domain.KocActive},
		bson.M{"$set": bson.M{"status": domain.KocArchived, "updatedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&koc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &koc, err
}

// DeleteAll removes every KOC (seeder only).
func (r *KocRepository) DeleteAll(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{})
	return err
}
