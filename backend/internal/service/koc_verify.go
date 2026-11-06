package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tiktoday/kocflow-api/internal/httpx"
)

// A KOC can only be added when TikTok Shop itself knows the creator: the channel must be listed in the Creator
// Marketplace ("Nhà sáng tạo" in Seller Center), found by its exact username. A typed-in nick that does not exist there
// (or only looks similar) is refused, so the directory never fills with accounts nobody can invite or pay.

// CreatorVerifier looks a creator up by its exact username (implemented by *TTShopService).
type CreatorVerifier interface {
	ResolveCreator(ctx context.Context, username string) (*TargetCreator, error)
}

// WithCreatorVerifier makes adding a KOC depend on the creator existing in TikTok Shop. Without one (tests, or an
// installation without TikTok Shop) nothing is checked.
func (s *KocService) WithCreatorVerifier(v CreatorVerifier) *KocService {
	s.verifier = v
	return s
}

// errCreatorNotListed is returned (as an AppError with status 422) when the username is not in the marketplace.
func errCreatorNotListed(tiktokID string) error {
	return httpx.NewError(http.StatusUnprocessableEntity,
		fmt.Sprintf("Không tìm thấy nhà sáng tạo @%s trong danh sách Nhà sáng tạo của TikTok Shop. Kiểm tra lại tên kênh (chính xác từng ký tự) rồi thử lại.", tiktokID))
}

// isNotListed reports whether err is the "not in the marketplace" refusal.
func isNotListed(err error) bool {
	var ae *httpx.AppError
	return errors.As(err, &ae) && ae.Status == http.StatusUnprocessableEntity
}

// verifyCreator returns the marketplace profile of the creator, or an error that says why the KOC cannot be added.
// It returns (nil, nil) only when no verifier is configured. Any failure to ask TikTok refuses the KOC too: the point
// is a guarantee, so "could not check" is never treated as "fine".
func (s *KocService) verifyCreator(ctx context.Context, tiktokID string) (*TargetCreator, error) {
	if s.verifier == nil {
		return nil, nil
	}
	c, err := s.verifier.ResolveCreator(ctx, tiktokID)
	switch {
	case err == nil:
		return c, nil
	case errors.Is(err, ErrCreatorNotFound):
		return nil, errCreatorNotListed(tiktokID)
	case errors.Is(err, ErrTTShopNotConnected):
		return nil, httpx.NewError(http.StatusConflict, "Chưa kết nối TikTok Shop nên chưa xác minh được nhà sáng tạo. Hãy ủy quyền shop rồi thử lại.")
	case IsRateLimited(err):
		return nil, httpx.NewError(http.StatusTooManyRequests, "TikTok đang giới hạn tốc độ nên chưa xác minh được nhà sáng tạo. Đợi khoảng một phút rồi thử lại.")
	}
	return nil, httpx.NewError(http.StatusBadGateway, "Chưa xác minh được nhà sáng tạo với TikTok Shop lúc này: "+err.Error())
}

// marketplaceProfile turns a marketplace creator into the profile shown when adding a KOC.
func marketplaceProfile(c *TargetCreator, tiktokID string) TikTokProfile {
	return TikTokProfile{TiktokID: tiktokID, DisplayName: c.Nickname, AvatarURL: c.AvatarURL, Followers: float64(c.Followers)}
}
