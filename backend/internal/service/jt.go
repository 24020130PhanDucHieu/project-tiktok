package service

import (
	"context"
	"errors"
	"time"
)

// JTTracking is the normalised result of a J&T Express lookup.
type JTTracking struct {
	Source     string `json:"source"`
	BillCode   string `json:"billCode"`
	Delivered  bool   `json:"delivered"`
	StatusText string `json:"statusText"`
	Details    []any  `json:"details,omitempty"`
	// Events is the journey of the parcel, newest scan first. Empty when the source gave no journey.
	Events []JTEvent `json:"events"`
}

// JTEvent is one scan of a parcel: when, and what J&T wrote about it.
type JTEvent struct {
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// Tracker looks up shipment status.
type Tracker interface {
	QueryTracking(ctx context.Context, billCode, phoneLast4 string) (*JTTracking, error)
}

// ErrPhoneLast4 is returned when the receiver's last 4 phone digits are missing or malformed.
var ErrPhoneLast4 = errors.New("Cần đúng 4 số cuối SĐT người nhận để tra cứu J&T")

// NormalizePhoneLast4 returns the last 4 digits of a phone number or of an already-4-digit string.
func NormalizePhoneLast4(s string) string {
	digits := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			digits = append(digits, s[i])
		}
	}
	if len(digits) < 4 {
		return ""
	}
	return string(digits[len(digits)-4:])
}
