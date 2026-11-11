package domain

// KocQuery is how the KOC directory is searched. Empty fields do not filter.
type KocQuery struct {
	Search   string // free text: name, phone, e-mail
	SearchID string // the same text as a normalised TikTok id (lower case, no "@")
	Category string
	Stage    string
	Source   string
}

// KocBrief is the little a list needs to name a KOC. It is its own type, not a half-filled Koc, so that nobody can
// save one back and wipe the fields that were never loaded.
type KocBrief struct {
	ID          ID     `bson:"_id"`
	TiktokID    string `bson:"tiktokId"`
	DisplayName string `bson:"displayName"`
	AvatarURL   string `bson:"avatarUrl"`
}
