package domain

import (
	"time"
)

// Collab statuses, modes, sample statuses and outcomes.
const (
	CollabActive = "active"
	CollabClosed = "closed"

	ModeTargeted = "targeted" // Mời cộng tác mục tiêu
	ModeOpen     = "open"     // Cộng tác mở (one link per product, pasted once)
	ModeProduct  = "product"  // Dán link sản phẩm

	SamplePending  = "pending"
	SampleApproved = "approved"
	SampleRejected = "rejected"

	OutcomePosted     = "posted"      // KOC đã đăng bài
	OutcomeNoPost     = "no_post"     // Nhận mẫu rồi không đăng
	OutcomeNoResponse = "no_response" // Không nhận mẫu / không phản hồi
	OutcomeNoNeed     = "no_need"     // Hết nhu cầu hợp tác
)

// CollabOutcomes lists every valid outcome.
var CollabOutcomes = []string{OutcomePosted, OutcomeNoPost, OutcomeNoResponse, OutcomeNoNeed}

// FeeChange is one entry of the cast-fee history.
type FeeChange struct {
	Fee float64   `bson:"fee" json:"fee"`
	At  time.Time `bson:"at" json:"at"`
}

// SampleRequest is a sample request from the KOC awaiting approval.
type SampleRequest struct {
	ID           string    `bson:"id" json:"id"`
	Product      string    `bson:"product" json:"product"`
	Status       string    `bson:"status" json:"status"`
	RejectReason string    `bson:"rejectReason,omitempty" json:"rejectReason,omitempty"`
	At           time.Time `bson:"at" json:"at"`
}

// CollabVideo is a video the KOC posted for this collaboration.
type CollabVideo struct {
	URL     string  `bson:"url" json:"url"`
	Title   string  `bson:"title" json:"title"`
	Views   float64 `bson:"views" json:"views"`
	Orders  float64 `bson:"orders" json:"orders"`
	Revenue float64 `bson:"revenue" json:"revenue"`
	AdsCode string  `bson:"adsCode,omitempty" json:"adsCode,omitempty"`
}

// CollabPerf is the "Hiệu quả" breakdown of a collaboration.
type CollabPerf struct {
	Revenue      float64 `json:"revenue"`
	Commission   float64 `json:"commission"`
	CastFee      float64 `json:"castFee"`
	Profit       float64 `json:"profit"`
	Approved     int     `json:"approved"`
	Videos       int     `json:"videos"`
	TotalViews   float64 `json:"totalViews"`
	CostPerMille float64 `json:"costPerMille"` // phí cast trên 1.000 lượt xem
	Orders       float64 `json:"orders"`
	// Source says where the figures come from: "tiktok" (real orders of this KOC, synced) or "manual" (typed in,
	// the collaboration has not been synced yet).
	Source              string  `json:"source"`
	CommissionConfirmed float64 `json:"commissionConfirmed"` // part of Commission that TikTok has already settled
}

// CollabReal is the snapshot of this collaboration's REAL sales read from TikTok Shop (affiliate orders of this KOC,
// for this collaboration's products, from the day it started). Once present it replaces the typed-in figures, so
// nothing is counted twice.
type CollabReal struct {
	SyncedAt            time.Time `bson:"syncedAt" json:"syncedAt"`
	Orders              int       `bson:"orders" json:"orders"`
	ItemsSold           int       `bson:"itemsSold" json:"itemsSold"`
	GMV                 float64   `bson:"gmv" json:"gmv"`
	Commission          float64   `bson:"commission" json:"commission"`
	CommissionConfirmed float64   `bson:"commissionConfirmed" json:"commissionConfirmed"`
	Excluded            int       `bson:"excluded" json:"excluded"` // orders left out (fully returned or ineligible)
	Videos              int       `bson:"videos" json:"videos"`
	Views               float64   `bson:"views" json:"views"`
	ViewsKnown          bool      `bson:"viewsKnown" json:"viewsKnown"`   // false when no counters could be read
	ViewsPublic         bool      `bson:"viewsPublic" json:"viewsPublic"` // views are the public counters of the video pages (no Analytics scope)
	ByProduct           bool      `bson:"byProduct" json:"byProduct"`     // filtered by the collaboration's product (else all of the KOC's sales)
	Truncated           bool      `bson:"truncated" json:"truncated"`
}

// Collab is one "lần hợp tác" with a KOC, from agreeing the fee to the video going live.
type Collab struct {
	ID             ID          `bson:"_id,omitempty" json:"_id"`
	KocID          ID          `bson:"kocId" json:"kocId"`
	Seq            int         `bson:"seq" json:"seq"`
	Name           string      `bson:"name" json:"name"`
	Status         string      `bson:"status" json:"status"`
	Step           int         `bson:"step" json:"step"` // 0 cast, 1 mode, 2 sample, 3 video, 4 result
	FeeSet         bool        `bson:"feeSet" json:"feeSet"`
	CastFee        float64     `bson:"castFee" json:"castFee"`
	FeeHistory     []FeeChange `bson:"feeHistory" json:"feeHistory"`
	FeePaid        bool        `bson:"feePaid" json:"feePaid"`
	Mode           string      `bson:"mode" json:"mode"`
	ProductName    string      `bson:"productName" json:"productName"`
	ProductID      string      `bson:"productId" json:"productId"` // TikTok Shop product id (open collaboration)
	ProductLink    string      `bson:"productLink" json:"productLink"`
	TargetCollabID string      `bson:"targetCollabId" json:"targetCollabId"` // TikTok target collaboration this collaboration was invited through
	ProductImage   string      `bson:"productImage" json:"productImage"`     // product picture from TikTok Shop (open collaboration)
	CommissionRate float64     `bson:"commissionRate" json:"commissionRate"`
	// AdsCommissionRate is the Shop Ads commission (percent) of an open collaboration. TikTok does not return it for
	// open collaborations, so a booker types it once per product (see OpenProduct).
	AdsCommissionRate float64         `bson:"adsCommissionRate" json:"adsCommissionRate"`
	Deadline          string          `bson:"deadline" json:"deadline"`
	FreeSample        bool            `bson:"freeSample" json:"freeSample"`
	Samples           []SampleRequest `bson:"samples" json:"samples"`
	Videos            []CollabVideo   `bson:"videos" json:"videos"`
	ShareOrders       float64         `bson:"shareOrders" json:"shareOrders"`
	ShareRevenue      float64         `bson:"shareRevenue" json:"shareRevenue"` // revenue outside TikTok (shared links...)
	Real              *CollabReal     `bson:"real,omitempty" json:"real"`
	Outcome           string          `bson:"outcome" json:"outcome"`
	StartedAt         time.Time       `bson:"startedAt" json:"startedAt"`
	EndedAt           *time.Time      `bson:"endedAt" json:"endedAt"`
	CreatedAt         time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time       `bson:"updatedAt" json:"updatedAt"`
	Perf              CollabPerf      `bson:"-" json:"perf"`
}

// Recompute derives Step and Perf from the collaboration data.
func (c *Collab) Recompute() {
	hasVideo := len(c.Videos) > 0 || (c.Real != nil && c.Real.Videos > 0)
	approved := 0
	for _, s := range c.Samples {
		if s.Status == SampleApproved {
			approved++
		}
	}
	switch {
	case !c.FeeSet:
		c.Step = 0
	case c.Mode == "":
		c.Step = 1
	case approved == 0 && !hasVideo:
		c.Step = 2
	case !hasVideo:
		c.Step = 3
	default:
		c.Step = 4
	}

	p := CollabPerf{CastFee: c.CastFee, Approved: approved, Videos: len(c.Videos), Revenue: c.ShareRevenue, Orders: c.ShareOrders, Source: "manual"}
	if r := c.Real; r != nil {
		// real orders from TikTok: the typed-in per-video figures are ignored so a sale is never counted twice;
		// only the revenue outside TikTok (ShareRevenue) is added, with its commission at the agreed rate.
		p.Source = "tiktok"
		p.Videos = r.Videos
		p.Revenue += r.GMV
		p.Orders += float64(r.Orders)
		if r.ViewsKnown {
			p.TotalViews = r.Views
		}
		p.Commission = r.Commission + c.ShareRevenue*c.CommissionRate/100
		p.CommissionConfirmed = r.CommissionConfirmed
	} else {
		for _, v := range c.Videos {
			p.Revenue += v.Revenue
			p.TotalViews += v.Views
			p.Orders += v.Orders
		}
		p.Commission = p.Revenue * c.CommissionRate / 100
	}
	p.Profit = p.Revenue - p.Commission - p.CastFee
	if p.TotalViews > 0 {
		p.CostPerMille = p.CastFee / p.TotalViews * 1000
	}
	c.Perf = p
}

// Owed returns the unpaid cast fee of a closed collaboration.
func (c *Collab) Owed() float64 {
	if c.Status == CollabClosed && !c.FeePaid && c.Outcome == OutcomePosted {
		return c.CastFee
	}
	return 0
}

// Normalize replaces nil slices so they serialise as [] instead of null.
func (c *Collab) Normalize() {
	if c.FeeHistory == nil {
		c.FeeHistory = []FeeChange{}
	}
	if c.Samples == nil {
		c.Samples = []SampleRequest{}
	}
	if c.Videos == nil {
		c.Videos = []CollabVideo{}
	}
}

// OpenProduct is what the team keeps about a product in open collaboration and TikTok's API does not give: the one
// link every creator uses for it, and its Shop Ads commission. Kept per product, so it is typed once and reused for
// every KOC.
type OpenProduct struct {
	ProductID         string    `bson:"_id" json:"productId"`
	Link              string    `bson:"link" json:"link"`
	AdsCommissionRate float64   `bson:"adsCommissionRate" json:"adsCommissionRate"` // percent
	UpdatedBy         string    `bson:"updatedBy" json:"updatedBy"`
	UpdatedAt         time.Time `bson:"updatedAt" json:"updatedAt"`
}
