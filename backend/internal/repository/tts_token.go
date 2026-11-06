package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ttsTokenDocID is the single document that holds the shop's authorization.
const ttsTokenDocID = "default"

// TTSTokenRepository stores the TikTok Shop authorization in the "tts_tokens" collection.
type TTSTokenRepository struct{ coll *mongo.Collection }

// NewTTSTokenRepository builds a TTSTokenRepository.
func NewTTSTokenRepository(db *mongo.Database) *TTSTokenRepository {
	return &TTSTokenRepository{coll: db.Collection("tts_tokens")}
}

// Load returns the stored authorization, or (nil, nil) when the shop has not authorized the app yet.
func (r *TTSTokenRepository) Load(ctx context.Context) (*domain.TTSToken, error) {
	var t domain.TTSToken
	err := r.coll.FindOne(ctx, bson.M{"_id": ttsTokenDocID}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Save replaces the stored authorization (the document id is always the same one).
func (r *TTSTokenRepository) Save(ctx context.Context, t *domain.TTSToken) error {
	t.ID = ttsTokenDocID
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": ttsTokenDocID}, t, options.Replace().SetUpsert(true))
	return err
}
