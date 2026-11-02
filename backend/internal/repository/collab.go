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

// CollabRepository persists Collab documents.
type CollabRepository struct{ coll *mongo.Collection }

// NewCollabRepository builds the repository on "collabs".
func NewCollabRepository(db *mongo.Database) *CollabRepository {
	return &CollabRepository{coll: db.Collection("collabs")}
}

// ListByKoc returns a KOC's collaborations, newest first.
func (r *CollabRepository) ListByKoc(ctx context.Context, kocID primitive.ObjectID) ([]domain.Collab, error) {
	cur, err := r.coll.Find(ctx, bson.M{"kocId": kocID}, options.Find().SetSort(bson.D{{Key: "seq", Value: -1}}))
	if err != nil {
		return nil, err
	}
	items := []domain.Collab{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Recompute()
	}
	return items, nil
}

// Active returns the open collaboration of a KOC, or (nil, nil).
func (r *CollabRepository) Active(ctx context.Context, kocID primitive.ObjectID) (*domain.Collab, error) {
	return r.findOne(ctx, bson.M{"kocId": kocID, "status": domain.CollabActive})
}

// FindByID returns a collaboration by hex id, or (nil, nil).
func (r *CollabRepository) FindByID(ctx context.Context, id string) (*domain.Collab, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil
	}
	return r.findOne(ctx, bson.M{"_id": oid})
}

func (r *CollabRepository) findOne(ctx context.Context, f bson.M) (*domain.Collab, error) {
	var c domain.Collab
	err := r.coll.FindOne(ctx, f).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Recompute()
	return &c, nil
}

// Create inserts a collaboration with the next sequence number for its KOC.
func (r *CollabRepository) Create(ctx context.Context, c *domain.Collab) error {
	n, err := r.coll.CountDocuments(ctx, bson.M{"kocId": c.KocID})
	if err != nil {
		return err
	}
	now := time.Now()
	c.Seq, c.Status, c.StartedAt, c.CreatedAt, c.UpdatedAt = int(n)+1, domain.CollabActive, now, now, now
	c.Normalize()
	res, err := r.coll.InsertOne(ctx, c)
	if err != nil {
		return err
	}
	c.ID = res.InsertedID.(primitive.ObjectID)
	c.Recompute()
	return nil
}

// Save replaces the stored collaboration (and refreshes derived fields).
func (r *CollabRepository) Save(ctx context.Context, c *domain.Collab) error {
	c.UpdatedAt = time.Now()
	c.Normalize()
	if _, err := r.coll.ReplaceOne(ctx, bson.M{"_id": c.ID}, c); err != nil {
		return err
	}
	c.Recompute()
	return nil
}
