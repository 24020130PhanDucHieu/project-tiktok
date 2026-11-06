package domain

import "time"

// TTSToken is the TikTok Shop authorization of the shop. Access and Refresh are stored encrypted (sealed): the
// repository only keeps them, it never sees the plain tokens.
type TTSToken struct {
	ID              string    `bson:"_id"`
	Access          string    `bson:"access"`  // AES-GCM sealed
	Refresh         string    `bson:"refresh"` // AES-GCM sealed
	AccessExpireAt  time.Time `bson:"accessExpireAt"`
	RefreshExpireAt time.Time `bson:"refreshExpireAt"`
	OpenID          string    `bson:"openId"`
	SellerName      string    `bson:"sellerName"`
	UpdatedAt       time.Time `bson:"updatedAt"`
}
