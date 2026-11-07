package service

import "time"

// TikTokVideoDetail is what TikTok shows publicly on a video's page: title, cover and the public counters.
// It is the same data any visitor sees, and it is what we fall back to when the Analytics scope is missing.
type TikTokVideoDetail struct {
	ID        string
	Title     string
	CoverURL  string
	CreatedAt time.Time
	Views     int64
	Likes     int64
	Comments  int64
	Shares    int64
	Username  string
}
