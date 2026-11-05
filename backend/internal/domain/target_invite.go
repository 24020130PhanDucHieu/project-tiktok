package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Statuses of a target invitation sent from this app.
const (
	TargetInviteSending   = "sending"   // request stored, TikTok not answered yet
	TargetInviteSent      = "sent"      // TikTok created it
	TargetInviteFailed    = "failed"    // TikTok refused it: nothing was created, safe to try again
	TargetInviteUnknown   = "unknown"   // connection broke: TikTok may or may not have created it, check before retrying
	TargetInviteCancelled = "cancelled" // cancelled from this app
)

// TargetInviteProduct is a product of a sent invitation.
type TargetInviteProduct struct {
	ID               string  `bson:"id" json:"id"`
	Title            string  `bson:"title" json:"title"`
	CommissionPct    float64 `bson:"commissionPct" json:"commissionPct"`
	AdsCommissionPct float64 `bson:"adsCommissionPct" json:"adsCommissionPct"`
}

// TargetInviteCreator is a creator that was invited.
type TargetInviteCreator struct {
	OpenID   string `bson:"openId" json:"openId"`
	Username string `bson:"username" json:"username"`
	Nickname string `bson:"nickname" json:"nickname"`
}

// TargetInvite is the local record of one invitation: who sent what to whom. It also makes sending idempotent
// (RequestKey is unique), so a double click can never send twice.
type TargetInvite struct {
	ID          primitive.ObjectID    `bson:"_id,omitempty" json:"id"`
	RequestKey  string                `bson:"requestKey" json:"requestKey"`
	Status      string                `bson:"status" json:"status"`
	Error       string                `bson:"error,omitempty" json:"error,omitempty"`
	TargetID    string                `bson:"targetId" json:"targetId"` // TikTok target collaboration id
	Name        string                `bson:"name" json:"name"`
	EndDate     string                `bson:"endDate" json:"endDate"`
	Message     string                `bson:"message" json:"message"`
	Products    []TargetInviteProduct `bson:"products" json:"products"`
	Creators    []TargetInviteCreator `bson:"creators" json:"creators"`
	KocID       string                `bson:"kocId,omitempty" json:"kocId,omitempty"`
	CollabID    string                `bson:"collabId,omitempty" json:"collabId,omitempty"`
	SentByID    string                `bson:"sentById" json:"sentById"`
	SentByEmail string                `bson:"sentByEmail" json:"sentByEmail"`
	CreatedAt   time.Time             `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time             `bson:"updatedAt" json:"updatedAt"`
}

// ShopContact is the shop's own contact shown to invited creators; it is remembered from the last invitation sent
// so the next invitation window starts filled in.
type ShopContact struct {
	Email    string `json:"email"`
	Zalo     string `json:"zalo"`
	Facebook string `json:"facebook"`
}
