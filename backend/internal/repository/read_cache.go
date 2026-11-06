package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// readCacheKeep is how long a cached answer stays in the database (the TTL index purges it afterwards).
const readCacheKeep = 48 * time.Hour

// ReadCacheRepository keeps the answers of TikTok Shop reads in the "tts_read_cache" collection, so a restart does not
// empty the cache.
type ReadCacheRepository struct{ coll *mongo.Collection }

// NewReadCacheRepository builds a ReadCacheRepository.
func NewReadCacheRepository(db *mongo.Database) *ReadCacheRepository {
	return &ReadCacheRepository{coll: db.Collection("tts_read_cache")}
}

type readCacheDoc struct {
	Key      string    `bson:"_id"`
	Data     []byte    `bson:"data"`
	At       time.Time `bson:"at"`
	ExpireAt time.Time `bson:"expireAt"`
}

// Load returns the stored answer and when it was stored; ok is false when there is none.
func (r *ReadCacheRepository) Load(ctx context.Context, key string) ([]byte, time.Time, bool, error) {
	var d readCacheDoc
	err := r.coll.FindOne(ctx, bson.M{"_id": key}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, err
	}
	return d.Data, d.At, true, nil
}

// Save stores (or replaces) the answer for key.
func (r *ReadCacheRepository) Save(ctx context.Context, key string, data []byte, at time.Time) error {
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": key},
		readCacheDoc{Key: key, Data: data, At: at, ExpireAt: at.Add(readCacheKeep)}, options.Replace().SetUpsert(true))
	return err
}
