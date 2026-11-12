package domain

import (
	"time"
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
	ID                ID        `bson:"_id,omitempty" json:"_id"`
	Channel           string    `bson:"channel" json:"channel"`
	ExternalContactID string    `bson:"externalContactId,omitempty" json:"externalContactId,omitempty"`
	ContactName       string    `bson:"contactName" json:"contactName"`
	ContactHandle     string    `bson:"contactHandle" json:"contactHandle"`
	ContactPhone      string    `bson:"contactPhone" json:"contactPhone"`
	AvatarURL         string    `bson:"avatarUrl" json:"avatarUrl"`
	LastMessage       string    `bson:"lastMessage" json:"lastMessage"`
	LastMessageAt     time.Time `bson:"lastMessageAt" json:"lastMessageAt"`
	UnreadCount       int       `bson:"unreadCount" json:"unreadCount"`
	Labels            []string  `bson:"labels,omitempty" json:"labels"`
	Hidden            bool      `bson:"hidden,omitempty" json:"hidden"`
	ChatbotOff        bool      `bson:"chatbotOff,omitempty" json:"chatbotOff"` // the chatbot does not answer in this conversation
	NextTask          string    `bson:"-" json:"nextTask"`
	Progress          string    `bson:"-" json:"progress,omitempty"` // where the KOC is when no action is waiting ("Đang giao mẫu")
	KocStage          string    `bson:"-" json:"stage,omitempty"`
	Status            string    `bson:"status" json:"status"`
	CreatedAt         time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time `bson:"updatedAt" json:"updatedAt"`
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
	ID        ID        `bson:"_id,omitempty" json:"_id"`
	Name      string    `bson:"name" json:"name"`
	Color     string    `bson:"color" json:"color"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
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
	ID             ID     `bson:"_id,omitempty" json:"_id"`
	ConversationID ID     `bson:"conversationId" json:"conversationId"`
	Direction      string `bson:"direction" json:"direction"`
	Kind           string `bson:"kind,omitempty" json:"kind,omitempty"`       // "system" for timeline notes
	Channel        string `bson:"channel,omitempty" json:"channel,omitempty"` // zalo | email
	To             string `bson:"to,omitempty" json:"to,omitempty"`
	Subject        string `bson:"subject,omitempty" json:"subject,omitempty"`
	ExternalID     string `bson:"externalId,omitempty" json:"-"`
	// What Zalo needs to react to a message the contact sent. Never shown to the browser: CanReact says it is possible.
	ZaloMsgID    string `bson:"zaloMsgId,omitempty" json:"-"`
	ZaloCliMsgID string `bson:"zaloCliMsgId,omitempty" json:"-"`
	Reaction     string `bson:"reaction,omitempty" json:"reaction,omitempty"` // key of our reaction (heart, like...), if any
	CanReact     bool   `bson:"-" json:"canReact,omitempty"`
	Text         string `bson:"text" json:"text"`
	Intent       string `bson:"intent,omitempty" json:"intent,omitempty"` // for a chatbot reply: what it answered (its memory of the chat)
	Status       string `bson:"status" json:"status"`
	// Proactive marks a message the system started on its own (invitation, reminder), as opposed to a reply: only these
	// count against the daily quota, and the mark is what lets the count survive a restart.
	Proactive bool      `bson:"proactive,omitempty" json:"-"`
	SentAt    time.Time `bson:"sentAt" json:"sentAt"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
