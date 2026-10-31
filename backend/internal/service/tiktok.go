package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
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

// TikTokService scrapes public TikTok endpoints.
type TikTokService struct {
	cfg  config.Config
	http *http.Client
}

// NewTikTokService builds a TikTokService.
func NewTikTokService(cfg config.Config) *TikTokService {
	return &TikTokService{cfg: cfg, http: &http.Client{}}
}

var (
	phoneRe    = regexp.MustCompile(`(?:\+84|0)[35789]\d{8}`)
	emailRe    = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	ogTitleRe  = regexp.MustCompile(`(?i)<meta property="og:title" content="([^"]+)"`)
	ogDescRe   = regexp.MustCompile(`(?i)<meta property="og:description" content="([^"]+)"`)
	ogImageRe  = regexp.MustCompile(`(?i)<meta property="og:image" content="([^"]+)"`)
	followerRe = regexp.MustCompile(`(?i)([\d.]+[kmb]?)\s+Followers`)
	onTikTokRe = regexp.MustCompile(`(?i)\s+on TikTok$`)
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

// videoDate derives dd/MM from a TikTok video id (snowflake: timestamp in the high bits).
func videoDate(id string) string {
	if n, ok := new(big.Int).SetString(id, 10); ok {
		sec := new(big.Int).Rsh(n, 32)
		if sec.IsInt64() && sec.Int64() > 1_500_000_000 && sec.Int64() < 2_500_000_000 {
			return time.Unix(sec.Int64(), 0).Format("02/01")
		}
	}
	return time.Now().Format("02/01")
}

// flexString decodes a JSON string or number into a string.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// flexNumber decodes a JSON number or numeric string into a float64.
type flexNumber float64

func (f *flexNumber) UnmarshalJSON(b []byte) error {
	var n float64
	if json.Unmarshal(b, &n) == nil {
		*f = flexNumber(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, _ := strconv.ParseFloat(s, 64)
	*f = flexNumber(v)
	return nil
}

type embedUser struct {
	UniqueID       string     `json:"uniqueId"`
	Nickname       string     `json:"nickname"`
	AvatarThumbURL string     `json:"avatarThumbUrl"`
	FollowerCount  flexNumber `json:"followerCount"`
	HeartCount     flexNumber `json:"heartCount"`
	Signature      string     `json:"signature"`
}

type embedVideo struct {
	ID             flexString `json:"id"`
	Desc           string     `json:"desc"`
	CoverURL       string     `json:"coverUrl"`
	OriginCoverURL string     `json:"originCoverUrl"`
	PlayCount      flexNumber `json:"playCount"`
}

type embedState struct {
	Source struct {
		Data map[string]struct {
			UserInfo  *embedUser   `json:"userInfo"`
			VideoList []embedVideo `json:"videoList"`
		} `json:"data"`
	} `json:"source"`
}

func (s *TikTokService) get(ctx context.Context, target string, timeout time.Duration, headers map[string]string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return body, resp.StatusCode, err
}

// FetchChannel reads profile + latest videos of a TikTok channel.
func (s *TikTokService) FetchChannel(ctx context.Context, rawHandle string) (*TikTokChannel, error) {
	uniqueID := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rawHandle), "@"))
	if uniqueID == "" {
		return nil, errors.New("Thiếu TikTok ID")
	}

	var profile *TikTokProfile
	videos := []domain.VideoItem{}

	// 1. TikTok embed endpoint (FRONTITY_CONNECT_STATE), retried because of overload protection.
attempts:
	for attempt := 0; attempt < 5; attempt++ {
		body, status, err := s.get(ctx, "https://www.tiktok.com/embed/@"+url.PathEscape(uniqueID), 10*time.Second, nil)
		if err == nil && status >= 200 && status < 300 {
			if p, v := parseEmbed(string(body), uniqueID); p != nil {
				profile, videos = p, v
				break attempts
			}
		}
		select {
		case <-ctx.Done():
			break attempts
		case <-time.After(time.Duration(450*(attempt+1)) * time.Millisecond):
		}
	}

	// 2. OpenGraph meta tags: canonical display name, HD avatar and follower fallback.
	if body, status, err := s.get(ctx, "https://www.tiktok.com/@"+url.PathEscape(uniqueID), 8*time.Second,
		map[string]string{"User-Agent": "facebookexternalhit/1.1"}); err == nil && status >= 200 && status < 300 {
		page := string(body)
		if d := ogDescRe.FindStringSubmatch(page); d != nil {
			desc := d[1]
			rawName := uniqueID
			if t := ogTitleRe.FindStringSubmatch(page); t != nil {
				rawName = t[1]
			}
			rawName = strings.TrimSpace(onTikTokRe.ReplaceAllString(rawName, ""))
			hdAvatar := ""
			if im := ogImageRe.FindStringSubmatch(page); im != nil {
				hdAvatar = strings.ReplaceAll(im[1], "&amp;", "&")
			}
			parsedFlw := 0.0
			if f := followerRe.FindStringSubmatch(desc); f != nil {
				parsedFlw = ParseCompactCount(f[1])
			}
			phone, email := ExtractContactFromBio(desc)

			if profile == nil {
				profile = &TikTokProfile{
					TiktokID: uniqueID, DisplayName: rawName, AvatarURL: hdAvatar, Followers: parsedFlw,
					VideoCount: len(videos), Signature: desc, Phone: phone, Email: email,
				}
			} else {
				if rawName != "" && rawName != uniqueID {
					profile.DisplayName = rawName
				}
				if hdAvatar != "" {
					profile.AvatarURL = hdAvatar
				}
				if profile.Followers == 0 && parsedFlw > 0 {
					profile.Followers = parsedFlw
				}
				if profile.Phone == "" && phone != "" {
					profile.Phone = phone
				}
				if profile.Email == "" && email != "" {
					profile.Email = email
				}
			}
		}
	}

	out := &TikTokChannel{Found: profile != nil, Videos: videos}
	if profile != nil {
		out.Profile = *profile
	} else {
		out.Profile = TikTokProfile{TiktokID: uniqueID, DisplayName: uniqueID}
	}
	return out, nil
}

func parseEmbed(page, uniqueID string) (*TikTokProfile, []domain.VideoItem) {
	marker := strings.Index(page, "FRONTITY_CONNECT_STATE")
	if marker == -1 {
		return nil, nil
	}
	gt := strings.Index(page[marker:], ">")
	if gt == -1 {
		return nil, nil
	}
	start := marker + gt + 1
	end := strings.Index(page[start:], "</script>")
	if end == -1 {
		return nil, nil
	}
	var state embedState
	if err := json.Unmarshal([]byte(page[start:start+end]), &state); err != nil {
		return nil, nil
	}
	for _, entry := range state.Source.Data {
		u := entry.UserInfo
		if u == nil || u.UniqueID == "" {
			continue
		}
		phone, email := ExtractContactFromBio(u.Signature)
		name := u.Nickname
		if name == "" {
			name = u.UniqueID
		}
		profile := &TikTokProfile{
			TiktokID: u.UniqueID, DisplayName: name, AvatarURL: u.AvatarThumbURL,
			Followers: nonNegative(float64(u.FollowerCount)), HeartCount: nonNegative(float64(u.HeartCount)),
			VideoCount: len(entry.VideoList), Signature: u.Signature, Phone: phone, Email: email,
		}
		list := entry.VideoList
		if len(list) > 15 {
			list = list[:15]
		}
		videos := make([]domain.VideoItem, 0, len(list))
		for _, v := range list {
			id := string(v.ID)
			cover := v.CoverURL
			if cover == "" {
				cover = v.OriginCoverURL
			}
			title := v.Desc
			if title == "" {
				title = "Video TikTok"
			}
			product := "Video trên kênh"
			if strings.Contains(v.Desc, "#") {
				parts := strings.Split(v.Desc, "#")[1:]
				if len(parts) > 2 {
					parts = parts[:2]
				}
				tags := make([]string, len(parts))
				for i, p := range parts {
					tags[i] = "#" + strings.TrimSpace(p)
				}
				product = strings.Join(tags, " ")
			}
			videos = append(videos, domain.VideoItem{
				ID: id, Date: videoDate(id), Title: title,
				URL:      fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", firstNonEmpty(u.UniqueID, uniqueID), id),
				CoverURL: cover, Views: FormatCompact(float64(v.PlayCount)),
				Likes: "—", Comments: "—", Shares: "—", Product: product,
			})
		}
		return profile, videos
	}
	return nil, nil
}

func nonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// FetchVideoMetrics validates a video URL through the official oEmbed endpoint.
func (s *TikTokService) FetchVideoMetrics(ctx context.Context, videoURL string) (*TikTokVideoMetrics, error) {
	clean := strings.TrimSpace(videoURL)
	if clean == "" {
		return nil, errors.New("Thiếu đường dẫn video TikTok")
	}
	body, status, err := s.get(ctx, "https://www.tiktok.com/oembed?url="+url.QueryEscape(clean), 8*time.Second,
		map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/124.0.0.0"})
	if err == nil && status >= 200 && status < 300 {
		var o struct {
			EmbedProductID flexString `json:"embed_product_id"`
			Title          string     `json:"title"`
			AuthorName     string     `json:"author_name"`
			AuthorUniqueID string     `json:"author_unique_id"`
			ThumbnailURL   string     `json:"thumbnail_url"`
		}
		if json.Unmarshal(body, &o) == nil {
			return &TikTokVideoMetrics{
				VideoID: string(o.EmbedProductID), Title: firstNonEmpty(o.Title, "Video TikTok"),
				AuthorName: o.AuthorName, AuthorUniqueID: o.AuthorUniqueID, CoverURL: o.ThumbnailURL,
				PostedAt: time.Now(),
			}, nil
		}
	}
	return &TikTokVideoMetrics{Title: "Video hợp tác KOC", PostedAt: time.Now()}, nil
}
