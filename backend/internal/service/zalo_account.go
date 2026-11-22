package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// ErrZaloNoCredentials marks an account that has nothing to log in with yet: somebody has to scan its QR code.
var ErrZaloNoCredentials = errors.New("tài khoản Zalo này chưa có thông tin đăng nhập, cần quét QR")

// ZaloAccountStore keeps the Zalo accounts (*repository.ZaloAccountRepository).
type ZaloAccountStore interface {
	Create(ctx context.Context, a *domain.ZaloAccount) error
	Update(ctx context.Context, a *domain.ZaloAccount) error
	// Find returns (nil, nil) when there is no such account.
	Find(ctx context.Context, id domain.ID) (*domain.ZaloAccount, error)
	// FindByUID returns the account of a Zalo user id, or (nil, nil).
	FindByUID(ctx context.Context, uid string) (*domain.ZaloAccount, error)
	List(ctx context.Context) ([]domain.ZaloAccount, error)
	Delete(ctx context.Context, id domain.ID) error
}

// ZaloAccountUsage says how many conversations talk through an account (*repository.ConversationRepository).
type ZaloAccountUsage interface {
	CountByZaloAccount(ctx context.Context, accountID string) (int64, error)
}

// sealedCredentials is what is encrypted into ZaloAccount.Credentials.
type sealedCredentials struct {
	Cookie    json.RawMessage `json:"cookie"`
	IMEI      string          `json:"imei"`
	UserAgent string          `json:"userAgent"`
}

// ZaloAccountService keeps the personal Zalo accounts and, sealed, what the bridge needs to log each one in again.
type ZaloAccountService struct {
	cfg   config.Config
	store ZaloAccountStore
	usage ZaloAccountUsage // nil: nothing is known to depend on an account
}

// NewZaloAccountService builds a ZaloAccountService. The credentials are sealed with the key the other stored
// secrets (Gmail, TikTok Shop) already use: TOKEN_ENCRYPTION_KEY, or JWT_SECRET when unset.
func NewZaloAccountService(cfg config.Config, store ZaloAccountStore) *ZaloAccountService {
	return &ZaloAccountService{cfg: cfg, store: store}
}

func (s *ZaloAccountService) gcm() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("zalo-account:" + s.cfg.TokenSecret()))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *ZaloAccountService) seal(plain []byte) (string, error) {
	gcm, err := s.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}

func (s *ZaloAccountService) open(sealed string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return nil, err
	}
	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("thông tin đăng nhập Zalo không hợp lệ")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}

func (s *ZaloAccountService) sealCreds(c ZaloCredentials) (string, error) {
	raw, err := json.Marshal(sealedCredentials{Cookie: c.Cookie, IMEI: c.IMEI, UserAgent: c.UserAgent})
	if err != nil {
		return "", err
	}
	return s.seal(raw)
}

// Create adds an account that has no credentials yet; they arrive once somebody scans its QR code.
func (s *ZaloAccountService) Create(ctx context.Context, label string) (*domain.ZaloAccount, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, domain.Invalid("Cần đặt tên cho tài khoản Zalo.")
	}
	now := time.Now()
	a := &domain.ZaloAccount{Label: label, CreatedAt: now, UpdatedAt: now}
	if err := s.store.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Get returns one account, or a not-found fault.
func (s *ZaloAccountService) Get(ctx context.Context, id domain.ID) (*domain.ZaloAccount, error) {
	a, err := s.store.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, domain.NotFound("Không tìm thấy tài khoản Zalo.")
	}
	return a, nil
}

// List returns every account, oldest first. The sealed credentials never leave the struct (json:"-").
func (s *ZaloAccountService) List(ctx context.Context) ([]domain.ZaloAccount, error) {
	return s.store.List(ctx)
}

// Default is the account that conversations stored before there were several belong to: the one marked default,
// otherwise the oldest. It is nil when there are no accounts.
func (s *ZaloAccountService) Default(ctx context.Context) (*domain.ZaloAccount, error) {
	all, err := s.store.List(ctx)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	for i := range all {
		if all[i].IsDefault {
			return &all[i], nil
		}
	}
	return &all[0], nil
}

// SaveCredentials stores what the bridge reported after a login for the account id. When the Zalo user is already
// stored under another account (the same person scanned a QR code again), that older account is updated and the
// new empty one is dropped, so one Zalo account is never two rows. It returns the account that holds the credentials.
func (s *ZaloAccountService) SaveCredentials(ctx context.Context, id domain.ID, zaloUID, displayName string, c ZaloCredentials) (*domain.ZaloAccount, error) {
	target, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if zaloUID != "" {
		existing, err := s.store.FindByUID(ctx, zaloUID)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != target.ID {
			if !target.HasCredentials() { // a placeholder created for this login: fold it into the account it turned out to be
				if err := s.store.Delete(ctx, target.ID); err != nil {
					return nil, err
				}
			}
			target = existing
		}
	}
	sealed, err := s.sealCreds(c)
	if err != nil {
		return nil, err
	}
	target.Credentials = sealed
	if zaloUID != "" {
		target.ZaloUID = zaloUID
	}
	if displayName != "" {
		target.DisplayName = displayName
	}
	target.UpdatedAt = time.Now()
	if err := s.store.Update(ctx, target); err != nil {
		return nil, err
	}
	return target, nil
}

// Credentials opens the sealed credentials of an account. ErrZaloNoCredentials means a QR login is needed.
func (s *ZaloAccountService) Credentials(ctx context.Context, id domain.ID) (*ZaloCredentials, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !a.HasCredentials() {
		return nil, ErrZaloNoCredentials
	}
	raw, err := s.open(a.Credentials)
	if err != nil {
		return nil, err
	}
	var c sealedCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &ZaloCredentials{Cookie: c.Cookie, IMEI: c.IMEI, UserAgent: c.UserAgent}, nil
}

// SetDisabled records that a person disconnected (or reconnected) the account: the automatic login must respect it.
func (s *ZaloAccountService) SetDisabled(ctx context.Context, id domain.ID, disabled bool) error {
	a, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	a.Disabled, a.UpdatedAt = disabled, time.Now()
	return s.store.Update(ctx, a)
}

// WithUsage tells the service where to count the conversations of an account, so that one that is still in use cannot
// be deleted.
func (s *ZaloAccountService) WithUsage(u ZaloAccountUsage) *ZaloAccountService {
	s.usage = u
	return s
}

// CanDelete says whether an account may be removed now: it must exist and no conversation may still talk through it
// (those could no longer be answered). When the conversations cannot be counted the answer is no.
func (s *ZaloAccountService) CanDelete(ctx context.Context, id domain.ID) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	if s.usage == nil {
		return nil
	}
	n, err := s.usage.CountByZaloAccount(ctx, id.Hex())
	if err != nil {
		return err
	}
	if n > 0 {
		return domain.Conflict(fmt.Sprintf("Tài khoản này còn %d cuộc trò chuyện nên chưa xóa được: xóa hoặc chuyển các cuộc trò chuyện đó trước.", n))
	}
	return nil
}

// Delete removes an account and its credentials, once CanDelete agrees.
func (s *ZaloAccountService) Delete(ctx context.Context, id domain.ID) error {
	if err := s.CanDelete(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// SeedDefault turns the account of ZALO_PERSONAL_* into the default account, once: only when there are no accounts
// yet and the environment holds a complete set. Later starts find accounts and leave everything alone.
func (s *ZaloAccountService) SeedDefault(ctx context.Context) error {
	existing, err := s.store.List(ctx)
	if err != nil || len(existing) > 0 {
		return err
	}
	if s.cfg.ZaloPersonalCookie == "" || s.cfg.ZaloPersonalIMEI == "" || s.cfg.ZaloPersonalUserAgent == "" {
		return nil
	}
	sealed, err := s.sealCreds(ZaloCredentials{Cookie: json.RawMessage(s.cfg.ZaloPersonalCookie), IMEI: s.cfg.ZaloPersonalIMEI, UserAgent: s.cfg.ZaloPersonalUserAgent})
	if err != nil {
		return err
	}
	now := time.Now()
	return s.store.Create(ctx, &domain.ZaloAccount{Label: "Tài khoản chính", IsDefault: true, Credentials: sealed, CreatedAt: now, UpdatedAt: now})
}
