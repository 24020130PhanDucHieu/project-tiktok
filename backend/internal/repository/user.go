package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// UserRepository persists User documents.
type UserRepository struct{ coll *mongo.Collection }

// NewUserRepository builds the repository on the "users" collection.
func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{coll: db.Collection("users")}
}

// FindByEmail returns the user with the (already normalised) email, or (nil, nil).
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &u, err
}

// Create inserts a user.
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	res, err := r.coll.InsertOne(ctx, u)
	if err != nil {
		return err
	}
	u.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// TouchLogin records the last login time.
func (r *UserRepository) TouchLogin(ctx context.Context, u *domain.User) error {
	now := time.Now()
	u.LastLoginAt = &now
	_, err := r.coll.UpdateByID(ctx, u.ID, bson.M{"$set": bson.M{"lastLoginAt": now, "updatedAt": now}})
	return err
}

// DeleteAll removes every user (seeder only).
func (r *UserRepository) DeleteAll(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{})
	return err
}
