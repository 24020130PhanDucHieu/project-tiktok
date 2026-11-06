package domain

// KocQuery is how the KOC directory is searched. Empty fields do not filter.
type KocQuery struct {
	Search   string // free text: name, phone, e-mail
	SearchID string // the same text as a normalised TikTok id (lower case, no "@")
	Category string
	Stage    string
	Source   string
}
