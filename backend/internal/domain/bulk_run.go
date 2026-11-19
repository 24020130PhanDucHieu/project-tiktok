package domain

import "time"

// Status of a bulk run.
const (
	RunRunning = "running"
	RunDone    = "done"
	RunStopped = "stopped"
)

// Status of one KOC inside a run.
const (
	TargetQueued  = "queued"   // not reached yet
	TargetSending = "sending"  // the steps are going out now
	TargetSent    = "sent"     // every step went out
	TargetReplied = "replied"  // the KOC answered in the middle: the rest was not sent
	TargetPartial = "partial"  // the run was stopped in the middle of this KOC's steps; StepsSent says how far
	TargetSkipped = "skipped"  // not eligible; Reason says why
	TargetFailed  = "failed"   // Zalo refused a step
	TargetUnknown = "unknown"  // a step may or may not have gone out: nobody can tell, check by hand
	TargetNotSent = "not_sent" // the run stopped before this KOC; Reason says why
)

// RunTarget is one KOC of a run and how far the steps got.
type RunTarget struct {
	KocID     string     `bson:"kocId" json:"kocId"`
	KocName   string     `bson:"kocName" json:"kocName"`
	Handle    string     `bson:"handle" json:"handle"`
	Status    string     `bson:"status" json:"status"`
	Reason    string     `bson:"reason,omitempty" json:"reason,omitempty"`
	StepsSent int        `bson:"stepsSent" json:"stepsSent"`
	StartedAt *time.Time `bson:"startedAt,omitempty" json:"startedAt,omitempty"`
}

// BulkRun is one press of "Gửi": a copy of the steps taken at that moment and the KOCs to send them to, one after the
// other. At most one run is running at a time.
type BulkRun struct {
	ID           ID             `bson:"_id,omitempty" json:"_id"`
	CampaignName string         `bson:"campaignName" json:"campaignName"`
	Steps        []CampaignStep `bson:"steps" json:"steps"`
	Targets      []RunTarget    `bson:"targets" json:"targets"`
	Status       string         `bson:"status" json:"status"`
	StopReason   string         `bson:"stopReason,omitempty" json:"stopReason,omitempty"`
	By           string         `bson:"by" json:"by"`
	WorkflowID   string         `bson:"workflowId,omitempty" json:"workflowId,omitempty"` // the workflow (and block) that started this run, if any
	NodeID       string         `bson:"nodeId,omitempty" json:"nodeId,omitempty"`
	IgnoreWindow bool           `bson:"ignoreWindow,omitempty" json:"ignoreWindow,omitempty"` // an admin chose to send outside 08:30 to 20:30
	CreatedAt    time.Time      `bson:"createdAt" json:"createdAt"`
	NextAt       *time.Time     `bson:"nextAt,omitempty" json:"nextAt,omitempty"`
	FinishedAt   *time.Time     `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
}
