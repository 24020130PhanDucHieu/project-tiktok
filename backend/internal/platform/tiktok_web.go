// Package platform holds the adapters to outside systems (TikTok's public pages, J&T, the
// zalo-bridge sidecar). They implement ports declared in package service, which never imports this package.
package platform

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
	"github.com/tiktoday/kocflow-api/internal/service"
)

// TikTokWeb reads TikTok's public pages (profile embed, oEmbed, video page).
type TikTokWeb struct {
	cfg  config.Config
	http *http.Client
}

// NewTikTokWeb builds a TikTokWeb.
func NewTikTokWeb(cfg config.Config) *TikTokWeb {
	return &TikTokWeb{cfg: cfg, http: &http.Client{}}
}

var (
	ogTitleRe  = regexp.MustCompile(`(?i)<meta property="og:title" content="([^"]+)"`)
	ogDescRe   = regexp.MustCompile(`(?i)<meta property="og:description" content="([^"]+)"`)
	ogImageRe  = regexp.MustCompile(`(?i)<meta property="og:image" content="([^"]+)"`)
	followerRe = regexp.MustCompile(`(?i)([\d.]+[kmb]?)\s+Followers`)
	onTikTokRe = regexp.MustCompile(`(?i)\s+on TikTok$`)
)

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

func (s *TikTokWeb) get(ctx context.Context, target string, timeout time.Duration, headers map[string]string) ([]byte, int, error) {
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
func (s *TikTokWeb) FetchChannel(ctx context.Context, rawHandle string) (*service.TikTokChannel, error) {
	uniqueID := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rawHandle), "@"))
	if uniqueID == "" {
		return nil, errors.New("Thiếu TikTok ID")
	}

	var profile *service.TikTokProfile
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
				parsedFlw = service.ParseCompactCount(f[1])
			}
			phone, email := service.ExtractContactFromBio(desc)

			if profile == nil {
				profile = &service.TikTokProfile{
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

	out := &service.TikTokChannel{Found: profile != nil, Videos: videos}
	if profile != nil {
		out.Profile = *profile
	} else {
		out.Profile = service.TikTokProfile{TiktokID: uniqueID, DisplayName: uniqueID}
	}
	return out, nil
}

func parseEmbed(page, uniqueID string) (*service.TikTokProfile, []domain.VideoItem) {
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
		phone, email := service.ExtractContactFromBio(u.Signature)
		name := u.Nickname
		if name == "" {
			name = u.UniqueID
		}
		profile := &service.TikTokProfile{
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
				CoverURL: cover, Views: service.FormatCompact(float64(v.PlayCount)),
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
func (s *TikTokWeb) FetchVideoMetrics(ctx context.Context, videoURL string) (*service.TikTokVideoMetrics, error) {
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
			return &service.TikTokVideoMetrics{
				VideoID: string(o.EmbedProductID), Title: firstNonEmpty(o.Title, "Video TikTok"),
				AuthorName: o.AuthorName, AuthorUniqueID: o.AuthorUniqueID, CoverURL: o.ThumbnailURL,
				PostedAt: time.Now(),
			}, nil
		}
	}
	return &service.TikTokVideoMetrics{Title: "Video hợp tác KOC", PostedAt: time.Now()}, nil
}

var rehydrationRE = regexp.MustCompile(`(?s)<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)

// parseVideoPage reads the public video page (the JSON TikTok embeds for its own web app).
func parseVideoPage(page []byte, wantID string) (*service.TikTokVideoDetail, error) {
	m := rehydrationRE.FindSubmatch(page)
	if m == nil {
		return nil, errors.New("trang video TikTok không có dữ liệu")
	}
	var root struct {
		Scope map[string]json.RawMessage `json:"__DEFAULT_SCOPE__"`
	}
	if err := json.Unmarshal(m[1], &root); err != nil {
		return nil, err
	}
	var vd struct {
		ItemInfo struct {
			ItemStruct struct {
				ID         string     `json:"id"`
				Desc       string     `json:"desc"`
				CreateTime flexNumber `json:"createTime"`
				Video      struct {
					Cover       string `json:"cover"`
					OriginCover string `json:"originCover"`
				} `json:"video"`
				Author struct {
					UniqueID string `json:"uniqueId"`
				} `json:"author"`
				Stats struct {
					PlayCount    flexNumber `json:"playCount"`
					DiggCount    flexNumber `json:"diggCount"`
					CommentCount flexNumber `json:"commentCount"`
					ShareCount   flexNumber `json:"shareCount"`
				} `json:"stats"`
			} `json:"itemStruct"`
		} `json:"itemInfo"`
	}
	raw, ok := root.Scope["webapp.video-detail"]
	if !ok {
		return nil, errors.New("TikTok không trả về chi tiết video này")
	}
	if err := json.Unmarshal(raw, &vd); err != nil {
		return nil, err
	}
	it := vd.ItemInfo.ItemStruct
	if it.ID == "" || (wantID != "" && it.ID != wantID) {
		return nil, errors.New("TikTok trả về video khác với video yêu cầu")
	}
	d := &service.TikTokVideoDetail{
		ID: it.ID, Title: strings.TrimSpace(it.Desc), CoverURL: firstNonEmpty(it.Video.Cover, it.Video.OriginCover), Username: strings.ToLower(it.Author.UniqueID),
		Views: int64(it.Stats.PlayCount), Likes: int64(it.Stats.DiggCount), Comments: int64(it.Stats.CommentCount), Shares: int64(it.Stats.ShareCount),
	}
	if sec := int64(it.CreateTime); sec > 0 {
		d.CreatedAt = time.Unix(sec, 0)
	}
	return d, nil
}

// FetchVideoDetail reads the public page of one video (title, cover and the public counters).
func (s *TikTokWeb) FetchVideoDetail(ctx context.Context, videoURL string) (*service.TikTokVideoDetail, error) {
	clean := strings.TrimSpace(videoURL)
	if !strings.HasPrefix(clean, "https://www.tiktok.com/") {
		return nil, errors.New("đường dẫn video TikTok không hợp lệ")
	}
	body, status, err := s.get(ctx, clean, 10*time.Second, map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Accept-Language": "vi-VN,vi;q=0.9,en;q=0.8",
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, errors.New("TikTok không mở được trang video")
	}
	id := clean[strings.LastIndex(clean, "/")+1:]
	if i := strings.IndexAny(id, "?#"); i >= 0 {
		id = id[:i]
	}
	return parseVideoPage(body, id)
}
