package domain

import "time"

// BulkSendLog is what happened to one KOC in one bulk send, kept so that the people who sent it can follow it later.
// RunID ties together the batches of one press of "Gửi". The text of the message is not stored, only its beginning.
type BulkSendLog struct {
	ID      ID        `bson:"_id,omitempty" json:"id"`
	RunID   string    `bson:"runId" json:"runId"`
	KocID   string    `bson:"kocId" json:"kocId"`
	KocName string    `bson:"kocName" json:"kocName"`
	Handle  string    `bson:"handle" json:"handle"`
	Status  string    `bson:"status" json:"status"`
	Reason  string    `bson:"reason,omitempty" json:"reason,omitempty"`
	Preview string    `bson:"preview" json:"preview"`
	By      string    `bson:"by" json:"by"`
	At      time.Time `bson:"at" json:"at"`
}
