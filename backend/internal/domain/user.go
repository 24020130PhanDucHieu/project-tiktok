package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User is an authenticated operator of the workspace.
type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name         string             `bson:"name" json:"name"`
	Email        string             `bson:"email" json:"email"`
	PasswordHash string             `bson:"passwordHash" json:"-"`
	Role         string             `bson:"role" json:"role"`
	LastLoginAt  *time.Time         `bson:"lastLoginAt,omitempty" json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time          `bson:"updatedAt" json:"updatedAt"`
}
