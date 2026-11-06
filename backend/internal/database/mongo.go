// Package database owns the MongoDB connection and index bootstrap.
package database

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect opens a pooled MongoDB connection and verifies it with a ping.
func Connect(ctx context.Context, uri, dbName string) (*mongo.Client, *mongo.Database, error) {
	opts := options.Client().ApplyURI(uri).
		SetRegistry(Registry()).
		SetServerSelectionTimeout(5 * time.Second).
		SetMaxPoolSize(20).SetMinPoolSize(2).
		SetSocketTimeout(45 * time.Second)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, nil, err
	}
	log.Println("MongoDB connected")
	return client, client.Database(dbName), nil
}

// EnsureIndexes creates the indexes previously declared by the mongoose schemas.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	type spec struct {
		coll   string
		models []mongo.IndexModel
	}
	asc := func(k string) bson.D { return bson.D{{Key: k, Value: 1}} }
	specs := []spec{
		{"users", []mongo.IndexModel{{Keys: asc("email"), Options: options.Index().SetUnique(true)}}},
		{"kocs", []mongo.IndexModel{
			{Keys: asc("tiktokId"), Options: options.Index().SetUnique(true)},
			{Keys: asc("phone")}, {Keys: asc("daHopTac")}, {Keys: asc("dnc")},
			{Keys: asc("stage")}, {Keys: asc("callTaskPending")}, {Keys: asc("trackingCode")},
			{Keys: asc("stalledFlag")}, {Keys: asc("status")},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "stage", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "category", Value: 1}, {Key: "followers", Value: -1}}},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "daHopTac", Value: 1}, {Key: "stage", Value: 1}}},
		}},
		{"conversations", []mongo.IndexModel{
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "lastMessageAt", Value: -1}}},
			{Keys: asc("contactHandle")},
			{Keys: bson.D{{Key: "channel", Value: 1}, {Key: "externalContactId", Value: 1}}},
		}},
		{"audit_logs", []mongo.IndexModel{
			{Keys: bson.D{{Key: "action", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: asc("createdAt"), Options: options.Index().SetExpireAfterSeconds(180 * 24 * 3600)}, // keep 180 days
		}},
		{"password_resets", []mongo.IndexModel{
			{Keys: asc("tokenHash")}, {Keys: asc("userId")},
			{Keys: asc("expiresAt"), Options: options.Index().SetExpireAfterSeconds(0)}, // auto-purge expired tokens
		}},
		{"target_invites", []mongo.IndexModel{
			{Keys: asc("requestKey"), Options: options.Index().SetUnique(true)}, // one send per request, however many clicks
			{Keys: asc("targetId")},
		}},
		{"tts_read_cache", []mongo.IndexModel{
			{Keys: asc("expireAt"), Options: options.Index().SetExpireAfterSeconds(0)}, // purge old cached TikTok answers
		}},
		{"messages", []mongo.IndexModel{
			{Keys: bson.D{{Key: "conversationId", Value: 1}, {Key: "sentAt", Value: 1}}},
		}},
	}
	for _, s := range specs {
		if _, err := db.Collection(s.coll).Indexes().CreateMany(ctx, s.models); err != nil {
			return err
		}
	}
	return nil
}
