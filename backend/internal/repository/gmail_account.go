package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// GmailAccountRepository stores the linked Gmail mailboxes in the "gmail_accounts" collection (one per user).
type GmailAccountRepository struct{ coll *mongo.Collection }

// NewGmailAccountRepository builds a GmailAccountRepository.
func NewGmailAccountRepository(db *mongo.Database) *GmailAccountRepository {
	return &GmailAccountRepository{coll: db.Collection("gmail_accounts")}
}

// Save links (or relinks) a mailbox: the document id is the user id.
func (r *GmailAccountRepository) Save(ctx context.Context, a *domain.GmailAccount) error {
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": a.UserID}, a, options.Replace().SetUpsert(true))
	return err
}

// Find returns the user's linked mailbox, or (nil, nil) when there is none.
func (r *GmailAccountRepository) Find(ctx context.Context, userID string) (*domain.GmailAccount, error) {
	var a domain.GmailAccount
	err := r.coll.FindOne(ctx, bson.M{"_id": userID}).Decode(&a)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Delete unlinks the user's mailbox.
func (r *GmailAccountRepository) Delete(ctx context.Context, userID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": userID})
	return err
}

// ListHealthy returns every linked mailbox that Google has not rejected (for the reply poller).
func (r *GmailAccountRepository) ListHealthy(ctx context.Context) ([]domain.GmailAccount, error) {
	cur, err := r.coll.Find(ctx, bson.M{"broken": false})
	if err != nil {
		return nil, err
	}
	var out []domain.GmailAccount
	return out, cur.All(ctx, &out)
}

// MarkBroken flags a mailbox whose token Google rejected.
func (r *GmailAccountRepository) MarkBroken(ctx context.Context, userID string) error {
	_, err := r.coll.UpdateByID(ctx, userID, bson.M{"$set": bson.M{"broken": true}})
	return err
}
