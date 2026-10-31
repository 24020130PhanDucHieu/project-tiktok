package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Conversation channels and statuses.
const (
	ChannelZalo     = "zalo"
	ChannelInternal = "internal"

	ConversationOpen = "open"
)

// Message directions and statuses.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"

	MessagePending   = "pending"
	MessageSent      = "sent"
	MessageDelivered = "delivered"
	MessageFailed    = "failed"
)

// Conversation is a chat thread with a KOC.
type Conversation struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Channel           string             `bson:"channel" json:"channel"`
	ExternalContactID string             `bson:"externalContactId,omitempty" json:"externalContactId,omitempty"`
	ContactName       string             `bson:"contactName" json:"contactName"`
	ContactHandle     string             `bson:"contactHandle" json:"contactHandle"`
	ContactPhone      string             `bson:"contactPhone" json:"contactPhone"`
	AvatarURL         string             `bson:"avatarUrl" json:"avatarUrl"`
	LastMessage       string             `bson:"lastMessage" json:"lastMessage"`
	LastMessageAt     time.Time          `bson:"lastMessageAt" json:"lastMessageAt"`
	UnreadCount       int                `bson:"unreadCount" json:"unreadCount"`
	Status            string             `bson:"status" json:"status"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Message is a single chat entry inside a Conversation.
type Message struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	ConversationID primitive.ObjectID `bson:"conversationId" json:"conversationId"`
	Direction      string             `bson:"direction" json:"direction"`
	Text           string             `bson:"text" json:"text"`
	Status         string             `bson:"status" json:"status"`
	SentAt         time.Time          `bson:"sentAt" json:"sentAt"`
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updatedAt" json:"updatedAt"`
}
