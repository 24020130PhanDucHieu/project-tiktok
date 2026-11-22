package domain

import "time"

// ZaloAccount is one personal Zalo account the app can log in to and send from. The credentials that let the bridge log
// in again (cookie, device id, user agent) are stored sealed (AES-GCM): the repository only keeps them, and nothing
// that goes to the browser ever carries them.
type ZaloAccount struct {
	ID          ID     `bson:"_id,omitempty" json:"_id"`
	Label       string `bson:"label" json:"label"`
	ZaloUID     string `bson:"zaloUid,omitempty" json:"zaloUid,omitempty"` // Zalo's own id: tells the same account apart when it logs in again
	DisplayName string `bson:"displayName,omitempty" json:"displayName,omitempty"`
	IsDefault   bool   `bson:"isDefault,omitempty" json:"isDefault"` // conversations stored before there were several accounts belong to it
	// Disabled is a person's choice to disconnect the account: the automatic login leaves it alone, also after a restart.
	Disabled    bool      `bson:"disabled,omitempty" json:"disabled"`
	Credentials string    `bson:"credentials,omitempty" json:"-"` // sealed JSON of the cookie, imei and user agent
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

// HasCredentials says the account can be logged in without scanning a QR code.
func (a ZaloAccount) HasCredentials() bool { return a.Credentials != "" }
