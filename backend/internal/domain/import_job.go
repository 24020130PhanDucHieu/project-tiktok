package domain

import "time"

// Status of an import job.
const (
	ImportRunning = "running"
	ImportDone    = "done"
	ImportStopped = "stopped"
)

// Status of one row of an import job.
const (
	RowQueued       = "queued"       // not reached yet (also what a row of a stopped job keeps)
	RowAdded        = "added"        // the KOC was saved
	RowDuplicate    = "duplicate"    // an active KOC with the same channel or phone exists
	RowCollaborated = "collaborated" // the creator already collaborated with the shop: blocked
	RowNotListed    = "not_listed"   // TikTok Shop has no such creator
	RowFailed       = "failed"       // our own store failed for this row
	RowInvalid      = "invalid"      // the server refused this row's data before the job began; Reason says why
)

// ImportRow is one row of the Excel sheet: what the sheet said, and how the import ended for it.
type ImportRow struct {
	Line            int      `bson:"line" json:"line"` // the row number in the sheet (the header is row 1)
	TiktokID        string   `bson:"tiktokId" json:"tiktokId"`
	DisplayName     string   `bson:"displayName,omitempty" json:"displayName,omitempty"`
	Phone           string   `bson:"phone,omitempty" json:"phone,omitempty"`
	Email           string   `bson:"email,omitempty" json:"email,omitempty"`
	Recipient       string   `bson:"recipient,omitempty" json:"recipient,omitempty"`
	ShippingAddress string   `bson:"shippingAddress,omitempty" json:"shippingAddress,omitempty"`
	Category        string   `bson:"category,omitempty" json:"category,omitempty"`
	Followers       *float64 `bson:"followers,omitempty" json:"followers,omitempty"`
	Gmv             *float64 `bson:"gmv,omitempty" json:"gmv,omitempty"`
	CastFee         *float64 `bson:"castFee,omitempty" json:"castFee,omitempty"`
	Status          string   `bson:"status" json:"status"`
	Reason          string   `bson:"reason,omitempty" json:"reason,omitempty"`
}

// ImportJob is one press of "Nhập": the rows of the sheet and how far they got. A background worker goes through the
// rows one after the other and writes each answer here; the screen only reads this record. At most one job is running.
type ImportJob struct {
	ID            ID          `bson:"_id,omitempty" json:"_id"`
	Source        string      `bson:"source" json:"source"`
	Rows          []ImportRow `bson:"rows" json:"rows"`
	Status        string      `bson:"status" json:"status"`
	StopReason    string      `bson:"stopReason,omitempty" json:"stopReason,omitempty"`
	StopRequested bool        `bson:"stopRequested,omitempty" json:"stopRequested,omitempty"` // somebody pressed "Dừng"
	By            string      `bson:"by" json:"by"`
	Current       string      `bson:"current,omitempty" json:"current,omitempty"`     // the TikTok id being checked now
	WaitUntil     *time.Time  `bson:"waitUntil,omitempty" json:"waitUntil,omitempty"` // TikTok is limiting calls: the next try is at this time
	WaitAttempt   int         `bson:"waitAttempt,omitempty" json:"waitAttempt,omitempty"`
	WaitKind      string      `bson:"waitKind,omitempty" json:"waitKind,omitempty"` // why it waits: "rate_limited" or "unavailable"
	CreatedAt     time.Time   `bson:"createdAt" json:"createdAt"`
	FinishedAt    *time.Time  `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
}
