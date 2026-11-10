package domain

import (
	"time"
)

// Roles and account statuses.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	UserActive    = "active"
	UserSuspended = "suspended"
	// UserPending is a self-registered account waiting for an admin: it cannot sign in until it is made active.
	UserPending = "pending"
)

// User is an authenticated operator of the workspace.
type User struct {
	ID           ID     `bson:"_id,omitempty" json:"id"`
	Name         string `bson:"name" json:"name"`
	Email        string `bson:"email" json:"email"`
	PasswordHash string `bson:"passwordHash" json:"-"`
	Role         string `bson:"role" json:"role"`
	Status       string `bson:"status" json:"status"`
	// TokenVersion is embedded in every JWT; bumping it revokes all issued tokens
	// (password change/reset, suspension, role change).
	TokenVersion      int        `bson:"tokenVersion" json:"-"`
	FailedLogins      int        `bson:"failedLogins" json:"-"`
	LockedUntil       *time.Time `bson:"lockedUntil,omitempty" json:"lockedUntil,omitempty"`
	LastLoginAt       *time.Time `bson:"lastLoginAt,omitempty" json:"lastLoginAt,omitempty"`
	PasswordChangedAt *time.Time `bson:"passwordChangedAt,omitempty" json:"passwordChangedAt,omitempty"`
	CreatedAt         time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time  `bson:"updatedAt" json:"updatedAt"`
}

// IsActive reports whether the account may sign in.
func (u *User) IsActive() bool { return u.Status == "" || u.Status == UserActive }

// PasswordReset is a single-use, expiring password-reset token (only its hash is stored).
type PasswordReset struct {
	ID        ID         `bson:"_id,omitempty"`
	UserID    ID         `bson:"userId"`
	TokenHash string     `bson:"tokenHash"`
	ExpiresAt time.Time  `bson:"expiresAt"`
	Attempts  int        `bson:"attempts"`
	UsedAt    *time.Time `bson:"usedAt,omitempty"`
	CreatedAt time.Time  `bson:"createdAt"`
}

// AuditLog records a security-relevant action.
type AuditLog struct {
	ID         ID        `bson:"_id,omitempty" json:"id"`
	ActorID    string    `bson:"actorId" json:"actorId"`
	ActorEmail string    `bson:"actorEmail" json:"actorEmail"`
	Action     string    `bson:"action" json:"action"`
	TargetType string    `bson:"targetType,omitempty" json:"targetType,omitempty"`
	TargetID   string    `bson:"targetId,omitempty" json:"targetId,omitempty"`
	Detail     string    `bson:"detail,omitempty" json:"detail,omitempty"`
	IP         string    `bson:"ip,omitempty" json:"ip,omitempty"`
	CreatedAt  time.Time `bson:"createdAt" json:"createdAt"`
}
