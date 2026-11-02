package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Conversation channels and statuses.
const (
	ChannelZalo     = "zalo"
	ChannelInternal = "internal"
	ChannelEmail    = "email"

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
	Labels            []string           `bson:"labels,omitempty" json:"labels"`
	Hidden            bool               `bson:"hidden,omitempty" json:"hidden"`
	NextTask          string             `bson:"-" json:"nextTask"`
	Status            string             `bson:"status" json:"status"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Next-task tags shown at the end of an inbox row ("Việc của bạn").
const (
	TaskCastFee    = "chot-phi-cast"
	TaskSendLink   = "gui-link"
	TaskReviewSamp = "duyet-mau"
	TaskSendSample = "gui-mau"
	TaskPayCast    = "tra-phi-cast"
)

// TaskTags lists every valid next-task tag.
var TaskTags = []string{TaskCastFee, TaskSendLink, TaskReviewSamp, TaskSendSample, TaskPayCast}

// Label is a user-defined tag attachable to conversations.
type Label struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Name      string             `bson:"name" json:"name"`
	Color     string             `bson:"color" json:"color"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// ConversationFilter narrows the inbox list.
type ConversationFilter struct {
	Query  string
	Unread bool
	Hidden bool
	Labels []string
	Tasks  []string
}

// Message is a single chat entry inside a Conversation.
type Message struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	ConversationID primitive.ObjectID `bson:"conversationId" json:"conversationId"`
	Direction      string             `bson:"direction" json:"direction"`
	Kind           string             `bson:"kind,omitempty" json:"kind,omitempty"`       // "system" for timeline notes
	Channel        string             `bson:"channel,omitempty" json:"channel,omitempty"` // zalo | email
	To             string             `bson:"to,omitempty" json:"to,omitempty"`
	Subject        string             `bson:"subject,omitempty" json:"subject,omitempty"`
	ExternalID     string             `bson:"externalId,omitempty" json:"-"`
	Text           string             `bson:"text" json:"text"`
	Status         string             `bson:"status" json:"status"`
	SentAt         time.Time          `bson:"sentAt" json:"sentAt"`
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updatedAt" json:"updatedAt"`
}
