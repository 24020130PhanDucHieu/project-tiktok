package domain

// KocContact is what the inbox needs to show a KOC as a conversation.
type KocContact struct {
	Handle      string // "@" + TikTok id: the key of the conversation
	Name        string
	Phone       string
	LastMessage string
}

// ZaloContact is what a Zalo lookup tells about the person behind a conversation. Only these fields are written, on
// their own, so a lookup cannot undo anything else that changed on the conversation meanwhile.
type ZaloContact struct {
	Channel           string
	ExternalContactID string
	Phone             string // "" leaves the stored phone as it is
	Name              string // "" leaves the stored name as it is
}
