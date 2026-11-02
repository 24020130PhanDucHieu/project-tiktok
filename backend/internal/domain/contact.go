package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Contact kinds.
const (
	ContactPhone = "phone"
	ContactEmail = "email"
)

// Contact is one way to reach a KOC (a Zalo phone number or an email address).
type Contact struct {
	ID          string    `bson:"id" json:"id"`
	Type        string    `bson:"type" json:"type"`
	Value       string    `bson:"value" json:"value"`
	Source      string    `bson:"source" json:"source"`
	Unreachable bool      `bson:"unreachable" json:"unreachable"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

// MessageTemplate is a reusable chat template; "{ten}" is replaced by the KOC name.
type MessageTemplate struct {
	ID   primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Name string             `bson:"name" json:"name"`
	Body string             `bson:"body" json:"body"`
}

// GmailAccount links a Gmail mailbox to a user (refresh token stored encrypted).
type GmailAccount struct {
	UserID       string    `bson:"_id" json:"-"`
	Email        string    `bson:"email" json:"email"`
	RefreshToken string    `bson:"refreshToken" json:"-"`
	Broken       bool      `bson:"broken" json:"broken"`
	UpdatedAt    time.Time `bson:"updatedAt" json:"updatedAt"`
}
