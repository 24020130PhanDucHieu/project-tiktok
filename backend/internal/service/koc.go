package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
)

// KocService holds KOC directory use-cases.
type KocService struct {
	kocs     KocDirectory
	convs    KocThreads
	outreach *OutreachService
	verifier CreatorVerifier // nil: creators are not checked against TikTok Shop (koc_verify.go)
}

// NewKocService builds a KocService.
func NewKocService(kocs KocDirectory, convs KocThreads, outreach *OutreachService) *KocService {
	return &KocService{kocs: kocs, convs: convs, outreach: outreach}
}

// NormalizeTikTokID lowercases and strips "@".
func NormalizeTikTokID(v string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "@"))
}

var spacesRe = regexp.MustCompile(`\s+`)

// NormalizePhone removes whitespace.
func NormalizePhone(v string) string { return strings.TrimSpace(spacesRe.ReplaceAllString(v, "")) }

// ResolveCastAndStage derives castType from castFee and promotes QUALIFIED to AWAITING_APPROVAL for paid casts.
func ResolveCastAndStage(castFee float64, requestedStage, currentStage string) (float64, string, string) {
	castType := "freecast"
	if castFee > 0 {
		castType = "cast"
	}
	stage := requestedStage
	if stage == "" {
		stage = currentStage
	}
	if stage == domain.StageQualified && castFee > 0 {
		stage = domain.StageAwaitingApproval
	}
	return castFee, castType, stage
}

// ListFilter are the directory query parameters.
type ListFilter struct {
	Q, Category, Stage, Source string
	Page, Limit                int64
}

// ListResult is a page of KOCs.
type ListResult struct {
	Items      []domain.Koc `json:"items"`
	Pagination Pagination   `json:"pagination"`
}

// Pagination describes a page window.
type Pagination struct {
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
	Total int64 `json:"total"`
	Pages int64 `json:"pages"`
}

// List returns the active KOC directory with optional filters.
func (s *KocService) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	q := domain.KocQuery{Search: f.Q, SearchID: NormalizeTikTokID(f.Q)}
	if f.Category != "Tất cả" {
		q.Category = f.Category
	}
	if f.Stage != "ALL" {
		q.Stage = f.Stage
	}
	if f.Source != "ALL" {
		q.Source = f.Source
	}
	page := max(f.Page, 1)
	limit := min(max(f.Limit, 1), 100)
	items, total, err := s.kocs.Search(ctx, q, (page-1)*limit, limit)
	if err != nil {
		return nil, err
	}
	pages := (total + limit - 1) / limit
	return &ListResult{Items: items, Pagination: Pagination{Page: page, Limit: limit, Total: total, Pages: pages}}, nil
}

// LookupResult is the payload of POST /kocs/lookup.
type LookupResult map[string]any

// Lookup checks the directory first, then the live TikTok channel. A nil result means "not found anywhere".
func (s *KocService) Lookup(ctx context.Context, tiktokID, phone string) (LookupResult, error) {
	tiktokID = NormalizeTikTokID(tiktokID)
	phone = NormalizePhone(phone)
	existing, err := s.kocs.FindByIdentity(ctx, tiktokID, phone)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.DaHopTac {
			return LookupResult{"source": "database", "blocked": true,
				"message": "Chặn liên hệ: KOC này đã từng hợp tác (da_hop_tac = true).", "profile": existing}, nil
		}
		msg := "Kênh đã lưu trữ trước đó."
		if existing.Status == domain.KocActive {
			msg = "Kênh hoặc SĐT này đã tồn tại trong danh bạ."
		}
		return LookupResult{"source": "database", "duplicate": existing.Status == domain.KocActive, "message": msg, "profile": existing}, nil
	}
	creator, err := s.verifyCreator(ctx, tiktokID)
	if err != nil {
		if isNotListed(err) {
			return LookupResult{"source": "tiktok-shop", "blocked": true, "verified": false, "message": err.(*httpx.AppError).Message}, nil
		}
		return nil, err
	}
	live, err := s.outreach.FetchChannel(ctx, tiktokID)
	if err != nil {
		return nil, err
	}
	if live.Found {
		profile := live.Profile
		if creator != nil && profile.DisplayName == "" {
			profile.DisplayName = creator.Nickname
		}
		msg := fmt.Sprintf("Đã lấy thông tin kênh thật @%s từ TikTok (%s follower).", live.Profile.TiktokID, FormatVND(live.Profile.Followers))
		if creator != nil {
			msg = fmt.Sprintf("Đã xác minh @%s có trong Nhà sáng tạo của TikTok Shop (%s follower).", tiktokID, FormatVND(profile.Followers))
		}
		return LookupResult{
			"source": "tiktok-live", "duplicate": false, "blocked": false, "verified": creator != nil,
			"message": msg, "profile": profile, "videos": live.Videos,
		}, nil
	}
	if creator != nil { // the public channel page could not be read, but TikTok Shop knows the creator
		return LookupResult{
			"source": "tiktok-shop", "duplicate": false, "blocked": false, "verified": true,
			"message": fmt.Sprintf("Đã xác minh @%s có trong Nhà sáng tạo của TikTok Shop (%s follower).", tiktokID, FormatVND(float64(creator.Followers))),
			"profile": marketplaceProfile(creator, tiktokID), "videos": []any{},
		}, nil
	}
	return nil, nil
}

// CreateInput is the validated payload to add a KOC.
type CreateInput struct {
	TiktokID, DisplayName, AvatarURL, Category, ContentPillar, TargetAudience, Source string
	Email, Phone, Recipient, ShippingAddress, Campaign, ProductName, Stage            string
	Followers                                                                         *int64
	Gmv, CommissionRate, CastFee                                                      *float64
	DaHopTac                                                                          *bool
}

// Create adds (or revives an archived) KOC, enriching it from the live TikTok channel.
func (s *KocService) Create(ctx context.Context, in CreateInput) (*domain.Koc, error) {
	tiktokID := NormalizeTikTokID(in.TiktokID)
	phone := NormalizePhone(in.Phone)

	existing, err := s.kocs.FindByIdentity(ctx, tiktokID, phone)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.DaHopTac {
		return nil, httpx.NewError(409, fmt.Sprintf("Chặn liên hệ: KOC @%s đã từng hợp tác (da_hop_tac = true)", existing.TiktokID))
	}
	if existing != nil && existing.Status == domain.KocActive {
		reason := "Số điện thoại " + phone
		if existing.TiktokID == tiktokID {
			reason = "TikTok ID @" + tiktokID
		}
		return nil, httpx.NewError(409, "Dedupe tự động: "+reason+" đã tồn tại trong danh bạ")
	}

	// The creator must exist in TikTok Shop before it is saved (see koc_verify.go).
	creator, err := s.verifyCreator(ctx, tiktokID)
	if err != nil {
		return nil, err
	}

	// Enrich from live TikTok data when the user did not look the channel up first.
	live, _ := s.outreach.FetchChannel(ctx, tiktokID)
	var liveProfile TikTokProfile
	if live != nil {
		liveProfile = live.Profile
	}
	if creator != nil { // what TikTok Shop says fills what the public page did not give
		mp := marketplaceProfile(creator, tiktokID)
		liveProfile.DisplayName = firstNonEmpty(liveProfile.DisplayName, mp.DisplayName)
		liveProfile.AvatarURL = firstNonEmpty(liveProfile.AvatarURL, mp.AvatarURL)
		if liveProfile.Followers == 0 {
			liveProfile.Followers = mp.Followers
		}
	}
	if phone == "" && liveProfile.Phone != "" {
		phone = NormalizePhone(liveProfile.Phone)
	}

	var castFee float64
	if in.CastFee != nil {
		castFee = *in.CastFee
	}
	fee, castType, stage := ResolveCastAndStage(castFee, in.Stage, orDefault(in.Stage, domain.StageSourced))

	wasNeverReplied := existing != nil && (existing.TungKhongRep || existing.Stage == domain.StageNoReply || existing.Stage == domain.StageDormant)

	k := domain.NewKoc()
	if existing != nil {
		k = existing
	}
	k.TiktokID = tiktokID
	switch {
	case in.DisplayName != "" && !strings.HasPrefix(in.DisplayName, "KOC @"):
		k.DisplayName = in.DisplayName
	case liveProfile.DisplayName != "":
		k.DisplayName = liveProfile.DisplayName
	case in.DisplayName != "":
		k.DisplayName = in.DisplayName
	default:
		k.DisplayName = "KOC @" + tiktokID
	}
	k.AvatarURL = firstNonEmpty(in.AvatarURL, liveProfile.AvatarURL)
	switch {
	case in.Followers != nil && *in.Followers > 0:
		k.Followers = float64(*in.Followers)
	default:
		k.Followers = liveProfile.Followers
	}
	k.Email = strings.ToLower(strings.TrimSpace(firstNonEmpty(in.Email, liveProfile.Email)))
	k.Phone = phone
	k.CastFee, k.CastType, k.Stage = fee, castType, stage
	k.TungKhongRep = wasNeverReplied
	k.Videos = []domain.VideoItem{}
	k.ChannelSyncedAt = nil
	if live != nil && len(live.Videos) > 0 {
		k.Videos = live.Videos
	}
	if live != nil && live.Found {
		k.ChannelSyncedAt = domain.Ptr(time.Now())
	}
	k.Status = domain.KocActive

	set := func(dst *string, v string) {
		if v != "" {
			*dst = strings.TrimSpace(v)
		}
	}
	set(&k.Category, in.Category)
	set(&k.ContentPillar, in.ContentPillar)
	set(&k.TargetAudience, in.TargetAudience)
	set(&k.Source, in.Source)
	set(&k.Recipient, in.Recipient)
	set(&k.ShippingAddress, in.ShippingAddress)
	set(&k.Campaign, in.Campaign)
	set(&k.ProductName, in.ProductName)
	if in.Gmv != nil {
		k.Gmv = *in.Gmv
	}
	if in.CommissionRate != nil {
		k.CommissionRate = *in.CommissionRate
	}
	if in.DaHopTac != nil {
		k.DaHopTac = *in.DaHopTac
	}

	if existing != nil {
		err = s.kocs.Save(ctx, k)
	} else {
		err = s.kocs.Create(ctx, k)
	}
	if err != nil {
		return nil, err
	}

	_, err = s.convs.UpsertKocContact(ctx, domain.KocContact{Handle: "@" + k.TiktokID, Name: k.DisplayName,
		Phone: k.Phone, LastMessage: "Đã thêm KOC vào danh bạ"})
	return k, err
}

// BulkItem is one row of a bulk import.
type BulkItem struct {
	TiktokID, DisplayName, Phone, Source, Category string
	Followers, Gmv, CastFee                        *float64
}

// BulkReport summarises a bulk import.
type BulkReport struct {
	Imported            []*domain.Koc    `json:"imported"`
	SkippedDuplicates   []map[string]any `json:"skippedDuplicates"`
	BlockedCollaborated []map[string]any `json:"blockedCollaborated"`
	NotVerified         []map[string]any `json:"notVerified"` // not in the Creator Marketplace, or TikTok could not be asked
}

// BulkImport imports many KOCs, skipping duplicates and blocking already-collaborated creators.
func (s *KocService) BulkImport(ctx context.Context, items []BulkItem, defaultSource string) (*BulkReport, error) {
	report := &BulkReport{Imported: []*domain.Koc{}, SkippedDuplicates: []map[string]any{}, BlockedCollaborated: []map[string]any{}, NotVerified: []map[string]any{}}
	deref := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}
	for _, raw := range items {
		tiktokID := NormalizeTikTokID(raw.TiktokID)
		if tiktokID == "" {
			continue
		}
		phone := NormalizePhone(raw.Phone)
		displayName := strings.TrimSpace(orDefault(raw.DisplayName, "KOC @"+tiktokID))
		source := orDefault(raw.Source, defaultSource)

		existing, err := s.kocs.FindByIdentity(ctx, tiktokID, phone)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.DaHopTac {
			report.BlockedCollaborated = append(report.BlockedCollaborated, map[string]any{"tiktokId": tiktokID, "phone": phone, "reason": "da_hop_tac = true"})
			continue
		}
		if existing != nil && existing.Status == domain.KocActive {
			reason := "Trùng SĐT"
			if existing.TiktokID == tiktokID {
				reason = "Trùng ID kênh"
			}
			report.SkippedDuplicates = append(report.SkippedDuplicates, map[string]any{"tiktokId": tiktokID, "phone": phone, "reason": reason})
			continue
		}

		if _, err := s.verifyCreator(ctx, tiktokID); err != nil {
			var ae *httpx.AppError
			if !errors.As(err, &ae) {
				return nil, err
			}
			report.NotVerified = append(report.NotVerified, map[string]any{"tiktokId": tiktokID, "phone": phone, "reason": ae.Message})
			continue
		}

		fee, castType, stage := ResolveCastAndStage(deref(raw.CastFee), "", domain.StageSourced)
		var saved *domain.Koc
		if existing != nil {
			saved = existing
			saved.TiktokID, saved.DisplayName, saved.Phone, saved.Source = tiktokID, displayName, phone, source
			saved.CastFee, saved.CastType, saved.Stage, saved.Status = fee, castType, stage, domain.KocActive
			err = s.kocs.Save(ctx, saved)
		} else {
			saved = domain.NewKoc()
			saved.TiktokID, saved.DisplayName, saved.Phone, saved.Source = tiktokID, displayName, phone, source
			saved.Category = orDefault(raw.Category, "Chưa phân loại")
			saved.Followers, saved.Gmv = deref(raw.Followers), deref(raw.Gmv)
			saved.CastFee, saved.CastType, saved.Stage = fee, castType, stage
			err = s.kocs.Create(ctx, saved)
		}
		if err != nil {
			return nil, err
		}
		if _, err := s.convs.EnsureKocContact(ctx, domain.KocContact{Handle: "@" + saved.TiktokID, Name: saved.DisplayName,
			Phone: saved.Phone, LastMessage: "Nhập hàng loạt từ " + source}); err != nil {
			return nil, err
		}
		report.Imported = append(report.Imported, saved)
	}
	return report, nil
}

// Archive soft-deletes a KOC.
func (s *KocService) Archive(ctx context.Context, id string) (*domain.Koc, error) {
	k, err := s.kocs.Archive(ctx, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, httpx.NewError(404, "Không tìm thấy KOC đang hoạt động")
	}
	return k, nil
}

// FindActive loads an active KOC or fails with 404.
func (s *KocService) FindActive(ctx context.Context, id string) (*domain.Koc, error) {
	k, err := s.kocs.FindActiveByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, httpx.NewError(404, "Không tìm thấy KOC đang hoạt động")
	}
	return k, nil
}

// Save persists a KOC.
func (s *KocService) Save(ctx context.Context, k *domain.Koc) error { return s.kocs.Save(ctx, k) }

// InviteAllQualified sends invitation #1 to every QUALIFIED KOC.
func (s *KocService) InviteAllQualified(ctx context.Context, brief string) ([]*domain.Koc, error) {
	list, err := s.kocs.FindQualifiedNotCollaborated(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Koc, 0, len(list))
	for _, k := range list {
		updated, err := s.outreach.SendFirstInvite(ctx, k, brief, true)
		if err != nil {
			return nil, err
		}
		out = append(out, updated)
	}
	return out, nil
}

// RecentActive returns the 100 newest active KOCs.
func (s *KocService) RecentActive(ctx context.Context) ([]domain.Koc, error) {
	return s.kocs.FindRecentActive(ctx, 100)
}
