package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// TikTokProfile is the public profile of a TikTok channel.
type TikTokProfile struct {
	TiktokID    string  `json:"tiktokId"`
	DisplayName string  `json:"displayName"`
	AvatarURL   string  `json:"avatarUrl"`
	Followers   float64 `json:"followers"`
	HeartCount  float64 `json:"heartCount"`
	VideoCount  int     `json:"videoCount"`
	Signature   string  `json:"signature,omitempty"`
	Phone       string  `json:"phone,omitempty"`
	Email       string  `json:"email,omitempty"`
}

// TikTokChannel is a looked-up channel together with its latest videos.
type TikTokChannel struct {
	Found   bool               `json:"found"`
	Profile TikTokProfile      `json:"profile"`
	Videos  []domain.VideoItem `json:"videos"`
}

// TikTokVideoMetrics is the oEmbed metadata of one video.
type TikTokVideoMetrics struct {
	VideoID        string    `json:"videoId"`
	Title          string    `json:"title"`
	AuthorName     string    `json:"authorName"`
	AuthorUniqueID string    `json:"authorUniqueId"`
	CoverURL       string    `json:"coverUrl"`
	PostedAt       time.Time `json:"postedAt"`
}

// TikTokClient reads public channel data.
type TikTokClient interface {
	FetchChannel(ctx context.Context, handle string) (*TikTokChannel, error)
	FetchVideoMetrics(ctx context.Context, videoURL string) (*TikTokVideoMetrics, error)
}

var (
	phoneRe = regexp.MustCompile(`(?:\+84|0)[35789]\d{8}`)
	emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
)

// ExtractContactFromBio pulls a Vietnamese phone number and an email out of a bio.
func ExtractContactFromBio(signature string) (phone, email string) {
	if m := phoneRe.FindString(signature); m != "" {
		phone = strings.TrimPrefix(m, "+84")
		if !strings.HasPrefix(m, "0") {
			phone = "0" + phone
		}
	}
	if m := emailRe.FindString(signature); m != "" {
		email = strings.ToLower(m)
	}
	return phone, email
}

// FormatCompact renders 12700 as "12.7k" and 1500000 as "1.5M".
func FormatCompact(n float64) string {
	trim := func(v float64) string {
		return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".0", "", 1)
	}
	switch {
	case n >= 1_000_000:
		return trim(n/1_000_000) + "M"
	case n >= 1_000:
		return trim(n/1_000) + "k"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// ParseCompactCount parses "1.2k", "3M", "2b" or a plain number.
func ParseCompactCount(s string) float64 {
	clean := strings.ToLower(strings.TrimSpace(s))
	i := 0
	for i < len(clean) && (clean[i] >= '0' && clean[i] <= '9' || clean[i] == '.') {
		i++
	}
	val, err := strconv.ParseFloat(clean[:i], 64)
	if err != nil {
		return 0
	}
	switch {
	case strings.HasSuffix(clean, "b"):
		val *= 1e9
	case strings.HasSuffix(clean, "m"):
		val *= 1e6
	case strings.HasSuffix(clean, "k"):
		val *= 1e3
	}
	return float64(int64(val + 0.5))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
