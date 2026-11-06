// Package domain holds the persistent entities of the platform.
package domain

import (
	"time"

)

// KOC pipeline stages.
const (
	StageSourced          = "SOURCED"
	StageQualified        = "QUALIFIED"
	StageAwaitingApproval = "AWAITING_APPROVAL"
	StageFilteredOut      = "FILTERED_OUT"
	StageContacted        = "CONTACTED"
	StageNoReply          = "NO_REPLY"
	StageReplied          = "REPLIED"
	StageDormant          = "DORMANT"
	StageAgreed           = "AGREED"
	StageDeclined         = "DECLINED"
	StageShipping         = "SHIPPING"
	StageDelivered        = "DELIVERED"
	StageVideoPosted      = "VIDEO_POSTED"
	StageNotFit           = "NOT_FIT"
	StageReturning        = "RETURNING"
	StageReturned         = "RETURNED"
	StageStalled          = "STALLED"
	StageCompleted        = "COMPLETED"
)

// KocStages lists every valid stage.
var KocStages = []string{
	StageSourced, StageQualified, StageAwaitingApproval, StageFilteredOut, StageContacted,
	StageNoReply, StageReplied, StageDormant, StageAgreed, StageDeclined, StageShipping,
	StageDelivered, StageVideoPosted, StageNotFit, StageReturning, StageReturned,
	StageStalled, StageCompleted,
}

// Koc statuses.
const (
	KocActive   = "active"
	KocArchived = "archived"
)

// VideoItem is a TikTok video scraped from the KOC channel.
type VideoItem struct {
	ID       string `bson:"id" json:"id"`
	Date     string `bson:"date" json:"date"`
	Title    string `bson:"title" json:"title"`
	URL      string `bson:"url" json:"url"`
	CoverURL string `bson:"coverUrl" json:"coverUrl"`
	Views    string `bson:"views" json:"views"`
	Likes    string `bson:"likes" json:"likes"`
	Comments string `bson:"comments" json:"comments"`
	Shares   string `bson:"shares" json:"shares"`
	Product  string `bson:"product" json:"product"`
}

// Koc is a creator tracked through the booking pipeline.
type Koc struct {
	ID                   ID `bson:"_id,omitempty" json:"_id"`
	TiktokID             string             `bson:"tiktokId" json:"tiktokId"`
	DisplayName          string             `bson:"displayName" json:"displayName"`
	AvatarURL            string             `bson:"avatarUrl" json:"avatarUrl"`
	Followers            float64            `bson:"followers" json:"followers"`
	Gmv                  float64            `bson:"gmv" json:"gmv"`
	Category             string             `bson:"category" json:"category"`
	ContentPillar        string             `bson:"contentPillar" json:"contentPillar"`
	TargetAudience       string             `bson:"targetAudience" json:"targetAudience"`
	Source               string             `bson:"source" json:"source"`
	Email                string             `bson:"email" json:"email"`
	Phone                string             `bson:"phone" json:"phone"`
	Recipient            string             `bson:"recipient" json:"recipient"`
	ShippingAddress      string             `bson:"shippingAddress" json:"shippingAddress"`
	DaHopTac             bool               `bson:"daHopTac" json:"daHopTac"`
	Dnc                  bool               `bson:"dnc" json:"dnc"`
	TungKhongRep         bool               `bson:"tungKhongRep" json:"tungKhongRep"`
	SuggestedDeclineCode string             `bson:"suggestedDeclineCode" json:"suggestedDeclineCode"`
	SelfShipAskedAt      *time.Time         `bson:"selfShipAskedAt" json:"selfShipAskedAt"`
	ReturnReminderSentAt *time.Time         `bson:"returnReminderSentAt" json:"returnReminderSentAt"`
	Campaign             string             `bson:"campaign" json:"campaign"`
	ProductName          string             `bson:"productName" json:"productName"`
	CommissionRate       float64            `bson:"commissionRate" json:"commissionRate"`
	CastType             string             `bson:"castType" json:"castType"`
	CastFee              float64            `bson:"castFee" json:"castFee"`
	Stage                string             `bson:"stage" json:"stage"`
	CollabStep           int                `bson:"collabStep" json:"collabStep"`
	RejectCode           string             `bson:"rejectCode" json:"rejectCode"`
	RejectNote           string             `bson:"rejectNote" json:"rejectNote"`

	// Stage 2 · outreach & follow-up
	InvitedAt       *time.Time `bson:"invitedAt" json:"invitedAt"`
	LastContactedAt *time.Time `bson:"lastContactedAt" json:"lastContactedAt"`
	LastRepliedAt   *time.Time `bson:"lastRepliedAt" json:"lastRepliedAt"`
	ReminderStep    int        `bson:"reminderStep" json:"reminderStep"`
	CallTaskPending bool       `bson:"callTaskPending" json:"callTaskPending"`
	DormantUntil    *time.Time `bson:"dormantUntil" json:"dormantUntil"`
	DeclineCode     string     `bson:"declineCode" json:"declineCode"`
	DeclineNote     string     `bson:"declineNote" json:"declineNote"`
	InviteBrief     string     `bson:"inviteBrief" json:"inviteBrief"`

	// Stage 3 · shipping
	ShippingCarrier      string     `bson:"shippingCarrier" json:"shippingCarrier"`
	TrackingCode         string     `bson:"trackingCode" json:"trackingCode"`
	TrackingPhoneLast4   string     `bson:"trackingPhoneLast4" json:"trackingPhoneLast4"`
	ShippedAt            *time.Time `bson:"shippedAt" json:"shippedAt"`
	DeliveredAt          *time.Time `bson:"deliveredAt" json:"deliveredAt"`
	LastTrackedAt        *time.Time `bson:"lastTrackedAt" json:"lastTrackedAt"`
	TrackingStatusText   string     `bson:"trackingStatusText" json:"trackingStatusText"`
	ShippingDelayedAlert bool       `bson:"shippingDelayedAlert" json:"shippingDelayedAlert"`

	// Stage 4 · video, GMV & returns
	VideoURL           string     `bson:"videoUrl" json:"videoUrl"`
	VideoPostedAt      *time.Time `bson:"videoPostedAt" json:"videoPostedAt"`
	VideoReminderStep  int        `bson:"videoReminderStep" json:"videoReminderStep"`
	KocVideoReply      string     `bson:"kocVideoReply" json:"kocVideoReply"`
	VideoViews         float64    `bson:"videoViews" json:"videoViews"`
	VideoOrders        float64    `bson:"videoOrders" json:"videoOrders"`
	VideoGmv           float64    `bson:"videoGmv" json:"videoGmv"`
	StalledFlag        bool       `bson:"stalledFlag" json:"stalledFlag"`
	PostReceiveCode    string     `bson:"postReceiveCode" json:"postReceiveCode"`
	PostReceiveNote    string     `bson:"postReceiveNote" json:"postReceiveNote"`
	ReturnTrackingCode string     `bson:"returnTrackingCode" json:"returnTrackingCode"`
	ReturnCondition    string     `bson:"returnCondition" json:"returnCondition"`
	ReturnedAt         *time.Time `bson:"returnedAt" json:"returnedAt"`

	Contacts        []Contact   `bson:"contacts" json:"contacts"`
	Videos          []VideoItem `bson:"videos" json:"videos"`
	ChannelSyncedAt *time.Time  `bson:"channelSyncedAt" json:"channelSyncedAt"`

	Status    string    `bson:"status" json:"status"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// NewKoc returns a Koc populated with the former mongoose schema defaults.
func NewKoc() *Koc {
	return &Koc{
		Category:        "Chưa phân loại",
		Source:          "TikToday",
		CommissionRate:  10,
		CastType:        "freecast",
		Stage:           StageSourced,
		ShippingCarrier: "J&T Express",
		Videos:          []VideoItem{},
		Status:          KocActive,
	}
}

// NextTask returns the pending owner action for this KOC ("" when nothing is waiting).
// CollabStep: 0 cast fee, 1 choose mode, 2 sample, 3 video, 4 result.
func (k *Koc) NextTask() string {
	if k.Status == KocArchived || k.Dnc || k.Stage == StageDeclined || k.Stage == StageNotFit {
		return ""
	}
	switch k.CollabStep {
	case 0:
		if k.Stage == StageReplied || k.Stage == StageAgreed {
			return TaskCastFee
		}
	case 1:
		return TaskSendLink
	case 2:
		if k.TrackingCode == "" && k.ShippingAddress != "" {
			return TaskSendSample
		}
		if k.TrackingCode == "" {
			return TaskReviewSamp
		}
	default:
		if k.CastFee > 0 && k.Stage == StageVideoPosted {
			return TaskPayCast
		}
	}
	return ""
}

// RaiseCollabStep lifts the collab step to at least n.
func (k *Koc) RaiseCollabStep(n int) {
	if n > k.CollabStep {
		k.CollabStep = n
	}
}

// Ptr returns a pointer to v (handy for optional timestamps).
func Ptr[T any](v T) *T { return &v }
