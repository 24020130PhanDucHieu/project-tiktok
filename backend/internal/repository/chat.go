package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ConversationRepository persists Conversation documents.
type ConversationRepository struct{ coll, labels, templates *mongo.Collection }

// NewConversationRepository builds the repository on "conversations" (+ "conversation_labels").
func NewConversationRepository(db *mongo.Database) *ConversationRepository {
	return &ConversationRepository{coll: db.Collection("conversations"), labels: db.Collection("conversation_labels"), templates: db.Collection("message_templates")}
}

// ListOpen returns open conversations matching the filter, newest message first.
// Task filtering is done by the caller because next tasks are derived from KOC data.
func (r *ConversationRepository) ListOpen(ctx context.Context, f domain.ConversationFilter) ([]domain.Conversation, error) {
	q := bson.M{"status": domain.ConversationOpen}
	if f.Hidden {
		q["hidden"] = true
	} else {
		q["hidden"] = bson.M{"$ne": true}
	}
	if f.Unread {
		q["unreadCount"] = bson.M{"$gt": 0}
	}
	if len(f.Labels) > 0 {
		q["labels"] = bson.M{"$in": f.Labels}
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(s), Options: "i"}
		q["$or"] = bson.A{bson.M{"contactName": re}, bson.M{"contactHandle": re}, bson.M{"contactPhone": re}}
	}
	cur, err := r.coll.Find(ctx, q, options.Find().SetSort(bson.D{{Key: "lastMessageAt", Value: -1}}).SetLimit(300))
	if err != nil {
		return nil, err
	}
	items := []domain.Conversation{}
	return items, cur.All(ctx, &items)
}

// ListTemplates returns every saved template, oldest first.
func (r *ConversationRepository) ListTemplates(ctx context.Context) ([]domain.MessageTemplate, error) {
	cur, err := r.templates.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	items := []domain.MessageTemplate{}
	return items, cur.All(ctx, &items)
}

// CreateTemplate inserts a template.
func (r *ConversationRepository) CreateTemplate(ctx context.Context, t *domain.MessageTemplate) error {
	res, err := r.templates.InsertOne(ctx, t)
	if err != nil {
		return err
	}
	t.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// DeleteTemplate removes a template.
func (r *ConversationRepository) DeleteTemplate(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil
	}
	_, err = r.templates.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

// BumpInbound records an inbound message on a thread: preview, unread +1, and un-hides it.
func (r *ConversationRepository) BumpInbound(ctx context.Context, id primitive.ObjectID, text string, at time.Time) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"lastMessage": text, "lastMessageAt": at, "hidden": false, "updatedAt": time.Now()},
		"$inc": bson.M{"unreadCount": 1},
	})
	return err
}

// MarkRead zeroes the unread counter.
func (r *ConversationRepository) MarkRead(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"unreadCount": 0}})
	return err
}

// SetChatbotOff turns the chatbot off (or back on) for one conversation.
func (r *ConversationRepository) SetChatbotOff(ctx context.Context, id primitive.ObjectID, off bool) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"chatbotOff": off, "updatedAt": time.Now()}})
	return err
}

// Delete removes the conversation itself (its messages are removed by the caller).
func (r *ConversationRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// SetHidden hides or restores a conversation without touching its messages.
func (r *ConversationRepository) SetHidden(ctx context.Context, id primitive.ObjectID, hidden bool) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"hidden": hidden, "updatedAt": time.Now()}})
	return err
}

// SetLabels replaces the label ids attached to a conversation.
func (r *ConversationRepository) SetLabels(ctx context.Context, id primitive.ObjectID, labels []string) error {
	if labels == nil {
		labels = []string{}
	}
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"labels": labels, "updatedAt": time.Now()}})
	return err
}

// ListLabels returns every label, oldest first.
func (r *ConversationRepository) ListLabels(ctx context.Context) ([]domain.Label, error) {
	cur, err := r.labels.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	items := []domain.Label{}
	return items, cur.All(ctx, &items)
}

// CreateLabel inserts a label.
func (r *ConversationRepository) CreateLabel(ctx context.Context, l *domain.Label) error {
	l.CreatedAt = time.Now()
	res, err := r.labels.InsertOne(ctx, l)
	if err != nil {
		return err
	}
	l.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// DeleteLabel removes a label and detaches it from every conversation.
func (r *ConversationRepository) DeleteLabel(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil
	}
	if _, err := r.labels.DeleteOne(ctx, bson.M{"_id": oid}); err != nil {
		return err
	}
	_, err = r.coll.UpdateMany(ctx, bson.M{"labels": id}, bson.M{"$pull": bson.M{"labels": id}})
	return err
}

// FindByHandle returns the thread of a KOC handle ("@tiktokId"), or (nil, nil).
func (r *ConversationRepository) FindByHandle(ctx context.Context, handle string) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.coll.FindOne(ctx, bson.M{"contactHandle": handle}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &c, err
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

// The three ways a KOC conversation is written. Each one is a plain function producing the "always apply" part and the
// "only when created" part, so the exact documents can be tested without a database.

// kocContactUpsert refreshes the whole conversation of a KOC that has just been added.
func kocContactUpsert(c domain.KocContact) (set, onInsert bson.M) {
	return bson.M{"channel": domain.ChannelInternal, "contactName": c.Name, "contactHandle": c.Handle,
		"contactPhone": c.Phone, "status": domain.ConversationOpen, "lastMessage": c.LastMessage}, bson.M{}
}

// kocContactEnsure only creates the conversation: an existing one is left as it is.
func kocContactEnsure(c domain.KocContact) (set, onInsert bson.M) {
	return bson.M{}, bson.M{"channel": domain.ChannelInternal, "contactName": c.Name, "contactHandle": c.Handle,
		"contactPhone": c.Phone, "status": domain.ConversationOpen, "lastMessage": c.LastMessage}
}

// kocMessageUpsert records the last message sent to a KOC; the channel is only set when the conversation is created.
func kocMessageUpsert(c domain.KocContact, at time.Time) (set, onInsert bson.M) {
	return bson.M{"contactName": c.Name, "contactHandle": c.Handle, "contactPhone": c.Phone,
			"status": domain.ConversationOpen, "lastMessage": c.LastMessage, "lastMessageAt": at},
		bson.M{"channel": domain.ChannelInternal}
}

// UpsertKocContact creates or refreshes the conversation of a KOC that has just been added.
func (r *ConversationRepository) UpsertKocContact(ctx context.Context, c domain.KocContact) (*domain.Conversation, error) {
	set, onInsert := kocContactUpsert(c)
	return r.upsertForKoc(ctx, c.Handle, set, onInsert)
}

// EnsureKocContact creates the conversation of a KOC when there is none and leaves an existing one untouched.
func (r *ConversationRepository) EnsureKocContact(ctx context.Context, c domain.KocContact) (*domain.Conversation, error) {
	set, onInsert := kocContactEnsure(c)
	return r.upsertForKoc(ctx, c.Handle, set, onInsert)
}

// UpsertKocMessage records the last message of the conversation with a KOC.
func (r *ConversationRepository) UpsertKocMessage(ctx context.Context, c domain.KocContact, at time.Time) (*domain.Conversation, error) {
	set, onInsert := kocMessageUpsert(c, at)
	return r.upsertForKoc(ctx, c.Handle, set, onInsert)
}

// upsertForKoc upserts the KOC conversation: `set` is always applied, `onInsert` only on creation.
func (r *ConversationRepository) upsertForKoc(ctx context.Context, handle string, set, onInsert bson.M) (*domain.Conversation, error) {
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

// FindByExternal returns the thread bound to this Zalo user id, or (nil, nil).
func (r *ConversationRepository) FindByExternal(ctx context.Context, externalID string) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.coll.FindOne(ctx, bson.M{"channel": domain.ChannelZalo, "externalContactId": externalID}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &c, err
}

func unlinkedKocCondition() bson.M {
	return bson.M{
		"contactHandle": bson.M{"$regex": "^@"},
		"contactPhone":  bson.M{"$ne": ""},
		"$or":           bson.A{bson.M{"externalContactId": bson.M{"$exists": false}}, bson.M{"externalContactId": ""}},
	}
}

// FindUnlinkedKocThreads returns the newest KOC threads that have a phone number but are not bound to a Zalo user yet.
func (r *ConversationRepository) FindUnlinkedKocThreads(ctx context.Context, limit int64) ([]domain.Conversation, error) {
	cur, err := r.coll.Find(ctx, unlinkedKocCondition(), options.Find().SetSort(bson.D{{Key: "lastMessageAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var out []domain.Conversation
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReleaseZalo frees the Zalo user id of a thread that only exists because the KOC could not be recognised, and hides
// it: the KOC's own thread takes the id over.
func (r *ConversationRepository) ReleaseZalo(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$unset": bson.M{"externalContactId": ""}, "$set": bson.M{"hidden": true, "updatedAt": time.Now()}})
	return err
}

// UpsertZaloInbound records an inbound Zalo message on its conversation and bumps unread count.
func (r *ConversationRepository) UpsertZaloInbound(ctx context.Context, externalID, name, text string, at time.Time) (*domain.Conversation, error) {
	// The name and handle are only written when the thread is created: a thread that belongs to a KOC (linked by its
	// phone number) keeps the KOC's name and "@handle" instead of being renamed after the Zalo display name.
	set := bson.M{
		"lastMessage": text, "lastMessageAt": at, "status": domain.ConversationOpen, "updatedAt": time.Now(), "hidden": false,
	}
	insert := insertDefaults(set, bson.M{"unreadCount": 0, "channel": 0, "contactName": name, "contactHandle": "zalo:" + externalID})
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

// Recent returns the newest n messages of a conversation, newest first (what the chatbot remembers of the chat).
func (r *MessageRepository) Recent(ctx context.Context, convID primitive.ObjectID, n int64) ([]domain.Message, error) {
	cur, err := r.coll.Find(ctx, bson.M{"conversationId": convID, "kind": bson.M{"$ne": "system"}},
		options.Find().SetSort(bson.D{{Key: "sentAt", Value: -1}}).SetLimit(n))
	if err != nil {
		return nil, err
	}
	var out []domain.Message
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByConversation returns messages oldest first.
func (r *MessageRepository) ListByConversation(ctx context.Context, convID, channel string) ([]domain.Message, error) {
	items := []domain.Message{}
	oid, err := primitive.ObjectIDFromHex(convID)
	if err != nil {
		return items, nil
	}
	// newest 300 messages only (index {conversationId, sentAt}); reversed back to chronological order
	filter := bson.M{"conversationId": oid}
	switch channel {
	case domain.ChannelEmail:
		filter["$or"] = bson.A{bson.M{"channel": domain.ChannelEmail}, bson.M{"kind": "system"}}
	case domain.ChannelZalo:
		filter["channel"] = bson.M{"$ne": domain.ChannelEmail}
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "sentAt", Value: -1}}).SetLimit(300))
	if err != nil {
		return nil, err
	}
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

// FindByID returns a message of a conversation, or (nil, nil).
func (r *MessageRepository) FindByID(ctx context.Context, convID primitive.ObjectID, id string) (*domain.Message, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil
	}
	var m domain.Message
	err = r.coll.FindOne(ctx, bson.M{"_id": oid, "conversationId": convID}).Decode(&m)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &m, err
}

// ExistsExternal reports whether an inbound message with this external id was already stored.
func (r *MessageRepository) ExistsExternal(ctx context.Context, externalID string) (bool, error) {
	n, err := r.coll.CountDocuments(ctx, bson.M{"externalId": externalID}, options.Count().SetLimit(1))
	return n > 0, err
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
