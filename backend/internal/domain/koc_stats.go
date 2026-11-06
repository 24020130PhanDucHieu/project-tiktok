package domain

// StageStat aggregates KOCs of one pipeline stage.
type StageStat struct {
	Stage    string  `bson:"_id" json:"stage"`
	Count    int64   `bson:"count" json:"count"`
	VideoGmv float64 `bson:"videoGmv" json:"videoGmv"`
	CastFee  float64 `bson:"castFee" json:"castFee"`
}

// BatchFunnel is the funnel of one campaign (booking batch), derived purely from timestamps.
type BatchFunnel struct {
	Campaign  string  `bson:"_id" json:"campaign"`
	Pipeline  int64   `bson:"pipeline" json:"pipeline"`
	Contacted int64   `bson:"contacted" json:"contacted"`
	Replied   int64   `bson:"replied" json:"replied"`
	Agreed    int64   `bson:"agreed" json:"agreed"`
	Delivered int64   `bson:"delivered" json:"delivered"`
	Posted    int64   `bson:"posted" json:"posted"`
	Gmv       float64 `bson:"gmv" json:"gmv"`
}

// TaskKoc is the slim projection shown in the "today" task list.
type TaskKoc struct {
	ID           string `bson:"_id" json:"id"`
	TiktokID     string `bson:"tiktokId" json:"tiktokId"`
	DisplayName  string `bson:"displayName" json:"displayName"`
	AvatarURL    string `bson:"avatarUrl" json:"avatarUrl"`
	Phone        string `bson:"phone" json:"phone"`
	Stage        string `bson:"stage" json:"stage"`
	TrackingCode string `bson:"trackingCode" json:"trackingCode"`
}

// TaskKind names one of the "today" task lists of the dashboard.
type TaskKind string

// The task lists of the dashboard.
const (
	TaskNeedCall         TaskKind = "needCall"
	TaskNeedApproval     TaskKind = "needApproval"
	TaskNeedShip         TaskKind = "needShip"
	TaskDelayedShipments TaskKind = "delayedShipments"
	TaskStalled          TaskKind = "stalled"
	TaskPendingReturn    TaskKind = "pendingReturn"
)

// AllTaskKinds lists the task lists in the order the dashboard shows them.
var AllTaskKinds = []TaskKind{TaskNeedCall, TaskNeedApproval, TaskNeedShip, TaskDelayedShipments, TaskStalled, TaskPendingReturn}
