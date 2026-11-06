package domain

// KocContact is what the inbox needs to show a KOC as a conversation.
type KocContact struct {
	Handle      string // "@" + TikTok id: the key of the conversation
	Name        string
	Phone       string
	LastMessage string
}
