package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

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

var rehydrationRE = regexp.MustCompile(`(?s)<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)

// parseVideoPage reads the public video page (the JSON TikTok embeds for its own web app).
func parseVideoPage(page []byte, wantID string) (*TikTokVideoDetail, error) {
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
	d := &TikTokVideoDetail{
		ID: it.ID, Title: strings.TrimSpace(it.Desc), CoverURL: firstNonEmpty(it.Video.Cover, it.Video.OriginCover), Username: strings.ToLower(it.Author.UniqueID),
		Views: int64(it.Stats.PlayCount), Likes: int64(it.Stats.DiggCount), Comments: int64(it.Stats.CommentCount), Shares: int64(it.Stats.ShareCount),
	}
	if sec := int64(it.CreateTime); sec > 0 {
		d.CreatedAt = time.Unix(sec, 0)
	}
	return d, nil
}

// FetchVideoDetail reads the public page of one video (title, cover and the public counters).
func (s *TikTokService) FetchVideoDetail(ctx context.Context, videoURL string) (*TikTokVideoDetail, error) {
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
