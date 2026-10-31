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

// ConversationRepository persists Conversation documents.
type ConversationRepository struct{ coll *mongo.Collection }

// NewConversationRepository builds the repository on "conversations".
func NewConversationRepository(db *mongo.Database) *ConversationRepository {
	return &ConversationRepository{coll: db.Collection("conversations")}
}

// ListOpen returns open conversations, newest message first.
func (r *ConversationRepository) ListOpen(ctx context.Context) ([]domain.Conversation, error) {
	cur, err := r.coll.Find(ctx, bson.M{"status": domain.ConversationOpen},
		options.Find().SetSort(bson.D{{Key: "lastMessageAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	items := []domain.Conversation{}
	return items, cur.All(ctx, &items)
}

// FindByID returns a conversation by hex id, or (nil, nil).
func (r *ConversationRepository) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil
	}
	var c domain.Conversation
	err = r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &c, err
}

// Save replaces the stored conversation.
func (r *ConversationRepository) Save(ctx context.Context, c *domain.Conversation) error {
	c.UpdatedAt = time.Now()
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": c.ID}, c)
	return err
}

// Create inserts a conversation.
func (r *ConversationRepository) Create(ctx context.Context, c *domain.Conversation) error {
	now := time.Now()
	c.CreatedAt, c.UpdatedAt = now, now
	if c.LastMessageAt.IsZero() {
		c.LastMessageAt = now
	}
	res, err := r.coll.InsertOne(ctx, c)
	if err != nil {
		return err
	}
	c.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// insertDefaults returns schema defaults for upserted conversations, minus keys the update already touches.
func insertDefaults(touched ...bson.M) bson.M {
	now := time.Now()
	d := bson.M{
		"channel": domain.ChannelZalo, "contactHandle": "", "contactPhone": "", "avatarUrl": "",
		"lastMessage": "", "lastMessageAt": now, "unreadCount": 0, "status": domain.ConversationOpen,
		"createdAt": now,
	}
	for _, t := range touched {
		for k := range t {
			delete(d, k)
		}
	}
	return d
}

// EnsureForKocs makes sure each KOC has a conversation (insert-only upsert).
func (r *ConversationRepository) EnsureForKocs(ctx context.Context, kocs []*domain.Koc) error {
	if len(kocs) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(kocs))
	for _, k := range kocs {
		handle := "@" + k.TiktokID
		onInsert := bson.M{
			"channel": domain.ChannelInternal, "contactName": k.DisplayName, "contactHandle": handle,
			"contactPhone": k.Phone, "lastMessage": "Đã thêm KOC vào danh bạ",
		}
		insert := insertDefaults(onInsert)
		for key, v := range onInsert {
			insert[key] = v
		}
		insert["updatedAt"] = time.Now()
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"contactHandle": handle}).
			SetUpdate(bson.M{"$setOnInsert": insert}).SetUpsert(true))
	}
	_, err := r.coll.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	return err
}

// UpsertForKoc upserts the KOC conversation: `set` is always applied, `onInsert` only on creation.
func (r *ConversationRepository) UpsertForKoc(ctx context.Context, handle string, set, onInsert bson.M) (*domain.Conversation, error) {
	set["updatedAt"] = time.Now()
	insert := insertDefaults(set, onInsert)
	for k, v := range onInsert {
		insert[k] = v
	}
	update := bson.M{"$set": set}
	if len(insert) > 0 {
		update["$setOnInsert"] = insert
	}
	return r.findOneAndUpsert(ctx, bson.M{"contactHandle": handle}, update)
}

// UpsertZaloInbound records an inbound Zalo message on its conversation and bumps unread count.
func (r *ConversationRepository) UpsertZaloInbound(ctx context.Context, externalID, name, text string, at time.Time) (*domain.Conversation, error) {
	set := bson.M{
		"contactName": name, "contactHandle": "zalo:" + externalID, "lastMessage": text,
		"lastMessageAt": at, "status": domain.ConversationOpen, "updatedAt": time.Now(),
	}
	insert := insertDefaults(set, bson.M{"unreadCount": 0, "channel": 0})
	update := bson.M{"$set": set, "$inc": bson.M{"unreadCount": 1}, "$setOnInsert": insert}
	// channel and externalContactId come from the equality filter on upsert.
	return r.findOneAndUpsert(ctx, bson.M{"channel": domain.ChannelZalo, "externalContactId": externalID}, update)
}

func (r *ConversationRepository) findOneAndUpsert(ctx context.Context, filter, update bson.M) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.coll.FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&c)
	return &c, err
}

// DeleteAll removes every conversation (seeder only).
func (r *ConversationRepository) DeleteAll(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{})
	return err
}

// SetLastMessage updates only the preview text of a conversation.
func (r *ConversationRepository) SetLastMessage(ctx context.Context, id primitive.ObjectID, text string) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"lastMessage": text, "updatedAt": time.Now()}})
	return err
}

// MessageRepository persists Message documents.
type MessageRepository struct{ coll *mongo.Collection }

// NewMessageRepository builds the repository on "messages".
func NewMessageRepository(db *mongo.Database) *MessageRepository {
	return &MessageRepository{coll: db.Collection("messages")}
}

// Create inserts a message; SentAt defaults to now.
func (r *MessageRepository) Create(ctx context.Context, m *domain.Message) error {
	now := time.Now()
	if m.SentAt.IsZero() {
		m.SentAt = now
	}
	m.CreatedAt, m.UpdatedAt = now, now
	res, err := r.coll.InsertOne(ctx, m)
	if err != nil {
		return err
	}
	m.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// Save replaces the stored message.
func (r *MessageRepository) Save(ctx context.Context, m *domain.Message) error {
	m.UpdatedAt = time.Now()
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": m.ID}, m)
	return err
}

// ListByConversation returns messages oldest first.
func (r *MessageRepository) ListByConversation(ctx context.Context, convID string) ([]domain.Message, error) {
	items := []domain.Message{}
	oid, err := primitive.ObjectIDFromHex(convID)
	if err != nil {
		return items, nil
	}
	cur, err := r.coll.Find(ctx, bson.M{"conversationId": oid}, options.Find().SetSort(bson.D{{Key: "sentAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return items, cur.All(ctx, &items)
}

// DeleteByConversation removes every message of a conversation.
func (r *MessageRepository) DeleteByConversation(ctx context.Context, convID primitive.ObjectID) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{"conversationId": convID})
	return err
}

// DeleteOne removes one message of a conversation.
func (r *MessageRepository) DeleteOne(ctx context.Context, convID, msgID string) error {
	c, err1 := primitive.ObjectIDFromHex(convID)
	m, err2 := primitive.ObjectIDFromHex(msgID)
	if err1 != nil || err2 != nil {
		return nil
	}
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": m, "conversationId": c})
	return err
}

// Latest returns the newest message of a conversation, or (nil, nil).
func (r *MessageRepository) Latest(ctx context.Context, convID primitive.ObjectID) (*domain.Message, error) {
	var m domain.Message
	err := r.coll.FindOne(ctx, bson.M{"conversationId": convID}, options.FindOne().SetSort(bson.D{{Key: "sentAt", Value: -1}})).Decode(&m)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &m, err
}

// DeleteAll removes every message (seeder only).
func (r *MessageRepository) DeleteAll(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{})
	return err
}

// InsertMany inserts seed messages.
func (r *MessageRepository) InsertMany(ctx context.Context, ms []*domain.Message) error {
	now := time.Now()
	docs := make([]any, len(ms))
	for i, m := range ms {
		if m.SentAt.IsZero() {
			m.SentAt = now.Add(time.Duration(i) * time.Millisecond)
		}
		m.CreatedAt, m.UpdatedAt = now, now
		docs[i] = m
	}
	_, err := r.coll.InsertMany(ctx, docs)
	return err
}
