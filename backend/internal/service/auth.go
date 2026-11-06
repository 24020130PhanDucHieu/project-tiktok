// Package service implements the business logic of the API.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
)

const (
	bcryptCost     = 12
	maxLoginFails  = 5
	lockDuration   = 15 * time.Minute
	userCacheTTL   = 15 * time.Second
	invalidLogin   = "Email hoặc mật khẩu không đúng"
	resetResponse  = "Nếu email tồn tại trong hệ thống, mã xác nhận 6 số đã được gửi tới hộp thư (kiểm tra cả mục Spam)."
	errAccountLock = "Tài khoản tạm khóa do nhập sai mật khẩu nhiều lần. Vui lòng thử lại sau ít phút."
)

// dummyHash equalises login timing for unknown emails.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("kocflow-timing-equaliser"), bcryptCost)

// AuthService registers, authenticates and manages user accounts.
type AuthService struct {
	users  UserStore
	resets ResetStore
	audit  *AuditService
	mailer Mailer
	cfg    config.Config

	mu    sync.Mutex
	cache map[string]cachedUser
}

type cachedUser struct {
	user *domain.User
	exp  time.Time
}

// NewAuthService builds an AuthService.
func NewAuthService(users UserStore, resets ResetStore, audit *AuditService, mailer Mailer, cfg config.Config) *AuthService {
	return &AuthService{users: users, resets: resets, audit: audit, mailer: mailer, cfg: cfg, cache: map[string]cachedUser{}}
}

// UserView is the public projection of a user.
type UserView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ViewOf projects a user.
func ViewOf(u *domain.User) UserView {
	return UserView{ID: u.ID.Hex(), Name: u.Name, Email: u.Email, Role: u.Role}
}

// Session is the login/register response payload.
type Session struct {
	Token string   `json:"token"`
	User  UserView `json:"user"`
}

// Claims are the JWT claims embedded in access tokens.
type Claims struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	TokenVersion int    `json:"tv"`
	jwt.RegisteredClaims
}

func (s *AuthService) session(u *domain.User) (*Session, error) {
	now := time.Now()
	claims := Claims{
		Role: u.Role, Name: u.Name, Email: u.Email, TokenVersion: u.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.Hex(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWTExpiresIn)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, User: ViewOf(u)}, nil
}

// ValidatePassword enforces the password policy.
func ValidatePassword(p string) error {
	var letter, digit bool
	for _, r := range p {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	if len([]rune(p)) < 8 || len(p) > 128 || !letter || !digit {
		return httpx.NewError(400, "Mật khẩu cần 8–128 ký tự, gồm cả chữ và số")
	}
	return nil
}

// Register creates a member account (only when public registration is enabled).
func (s *AuthService) Register(ctx context.Context, name, email, password string) (*Session, error) {
	if !s.cfg.AllowRegistration {
		return nil, httpx.NewError(403, "Đăng ký công khai đã tắt. Vui lòng liên hệ quản trị viên để được cấp tài khoản.")
	}
	u, err := s.CreateUser(ctx, name, email, password, domain.RoleMember)
	if err != nil {
		return nil, err
	}
	return s.session(u)
}

// CreateUser creates an account with a hashed password.
func (s *AuthService) CreateUser(ctx context.Context, name, email, password, role string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if existing, err := s.users.FindByEmail(ctx, email); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, httpx.NewError(409, "Email đã được sử dụng")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}
	u := &domain.User{Name: strings.TrimSpace(name), Email: email, PasswordHash: string(hash), Role: role, Status: domain.UserActive}
	return u, s.users.Create(ctx, u)
}

// Login verifies credentials, enforces lockout / suspension and issues a token.
func (s *AuthService) Login(ctx context.Context, email, password, ip string) (*Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		s.audit.Log(nil, "", email, "auth.login_failed", "", "", "unknown email", ip)
		return nil, httpx.NewError(401, invalidLogin)
	}
	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return nil, httpx.NewError(423, errAccountLock)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		_ = s.users.RecordLoginFailure(ctx, user, maxLoginFails, lockDuration)
		s.audit.Log(nil, user.ID.Hex(), user.Email, "auth.login_failed", "user", user.ID.Hex(), "wrong password", ip)
		return nil, httpx.NewError(401, invalidLogin)
	}
	if !user.IsActive() {
		return nil, httpx.NewError(403, "Tài khoản đã bị khóa. Vui lòng liên hệ quản trị viên.")
	}
	if err := s.users.RecordLoginSuccess(ctx, user.ID); err != nil {
		return nil, err
	}
	s.audit.Log(nil, user.ID.Hex(), user.Email, "auth.login", "", "", "", ip)
	return s.session(user)
}

// ParseToken validates the signature/expiry of a bearer token.
func (s *AuthService) ParseToken(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// Authenticate validates a token and checks the account is still active with a matching token version.
// Lookups are cached for a few seconds so authenticated requests rarely hit the database.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	claims, err := s.ParseToken(token)
	if err != nil {
		return nil, err
	}
	u, err := s.cachedUser(ctx, claims.Subject)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.IsActive() || u.TokenVersion != claims.TokenVersion {
		return nil, fmt.Errorf("session revoked")
	}
	return u, nil
}

func (s *AuthService) cachedUser(ctx context.Context, id string) (*domain.User, error) {
	s.mu.Lock()
	if c, ok := s.cache[id]; ok && time.Now().Before(c.exp) {
		s.mu.Unlock()
		return c.user, nil
	}
	s.mu.Unlock()
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if len(s.cache) > 2000 {
		s.cache = map[string]cachedUser{}
	}
	s.cache[id] = cachedUser{user: u, exp: time.Now().Add(userCacheTTL)}
	s.mu.Unlock()
	return u, nil
}

// Invalidate drops a user from the auth cache (after role/status/password changes).
func (s *AuthService) Invalidate(id string) {
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
}

// Profile returns the fresh account data.
func (s *AuthService) Profile(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, httpx.NewError(404, "Không tìm thấy tài khoản")
	}
	return u, nil
}

// UpdateName changes the display name.
func (s *AuthService) UpdateName(ctx context.Context, u *domain.User, name string) error {
	if err := s.users.Rename(ctx, u.ID, strings.TrimSpace(name)); err != nil {
		return err
	}
	s.Invalidate(u.ID.Hex())
	return nil
}

func (s *AuthService) setPassword(ctx context.Context, u *domain.User, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	now := time.Now()
	err = s.users.SetPassword(ctx, u.ID, string(hash), now) // also revokes every previously issued token
	s.Invalidate(u.ID.Hex())
	return err
}

// ChangePassword verifies the current password then sets a new one and returns a fresh session.
func (s *AuthService) ChangePassword(ctx context.Context, u *domain.User, current, next, ip string) (*Session, error) {
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(current)) != nil {
		return nil, httpx.NewError(400, "Mật khẩu hiện tại không đúng")
	}
	if current == next {
		return nil, httpx.NewError(400, "Mật khẩu mới phải khác mật khẩu hiện tại")
	}
	if err := s.setPassword(ctx, u, next); err != nil {
		return nil, err
	}
	s.audit.Log(nil, u.ID.Hex(), u.Email, "auth.password_changed", "user", u.ID.Hex(), "", ip)
	fresh, err := s.Profile(ctx, u.ID.Hex())
	if err != nil {
		return nil, err
	}
	return s.session(fresh)
}

// maxCodeAttempts bounds guessing of the 6-digit code (10^6 space, 5 tries per issued code).
const maxCodeAttempts = 5

// hashCode binds a verification code to its user so equal codes of different users never collide.
func hashCode(userID, code string) string {
	sum := sha256.Sum256([]byte(userID + ":" + code))
	return hex.EncodeToString(sum[:])
}

func sixDigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// SendResetCode e-mails a 6-digit verification code. The caller receives no hint whether the account exists.
func (s *AuthService) SendResetCode(ctx context.Context, email, ip string) {
	u, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || u == nil || !u.IsActive() {
		return
	}
	if err := s.IssueResetCode(ctx, u, false); err != nil {
		log.Printf("password reset mail failed for %s: %v", u.Email, err)
		return
	}
	s.audit.Log(nil, u.ID.Hex(), u.Email, "auth.reset_requested", "user", u.ID.Hex(), "", ip)
}

// IssueResetCode stores a hashed single-use code and e-mails it (invite=true: welcome wording, 24h validity).
func (s *AuthService) IssueResetCode(ctx context.Context, u *domain.User, invite bool) error {
	code, err := sixDigitCode()
	if err != nil {
		return err
	}
	ttl := s.cfg.PasswordResetTTL
	if invite {
		ttl = 24 * time.Hour
	}
	if err := s.resets.Create(ctx, &domain.PasswordReset{
		UserID: u.ID, TokenHash: hashCode(u.ID.Hex(), code), ExpiresAt: time.Now().Add(ttl),
	}); err != nil {
		return err
	}
	site := strings.TrimRight(s.cfg.FrontendURL, "/")
	validity := fmt.Sprintf("%d phút", int(ttl.Minutes()))
	if invite {
		validity = "24 giờ"
	}
	subject := fmt.Sprintf("%s là mã xác nhận KocFlow của bạn", code)
	intro := "Bạn vừa yêu cầu đặt lại mật khẩu tài khoản KocFlow."
	if invite {
		subject = "Chào mừng bạn đến với KocFlow - mã đặt mật khẩu " + code
		intro = "Quản trị viên đã tạo tài khoản KocFlow cho bạn. Hãy đặt mật khẩu để bắt đầu."
	}
	body := fmt.Sprintf("Chào %s,\n\n%s\n\nMã xác nhận của bạn:   %s\n\n"+
		"Mã có hiệu lực %s và chỉ dùng được một lần. Cách dùng:\n"+
		"1. Mở %s\n"+
		"2. Chọn \"Quên mật khẩu?\" (hoặc menu Tài khoản > Đổi mật khẩu bằng mã email)\n"+
		"3. Nhập email %s, mã ở trên và mật khẩu mới.\n\n"+
		"Không chia sẻ mã này với bất kỳ ai.\n"+
		"Nếu bạn không yêu cầu, hãy bỏ qua email này - mật khẩu hiện tại vẫn được giữ nguyên.\n\nKocFlow Studio",
		u.Name, intro, code, validity, site, u.Email)
	return s.mailer.Send(u.Email, subject, body)
}

// ResetPassword verifies an e-mailed code and sets a new password.
func (s *AuthService) ResetPassword(ctx context.Context, email, code, password, ip string) error {
	invalid := httpx.NewError(400, "Mã xác nhận không đúng hoặc đã hết hạn")
	if err := ValidatePassword(password); err != nil {
		return err
	}
	u, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if u == nil || !u.IsActive() {
		return invalid
	}
	rec, err := s.resets.Active(ctx, u.ID)
	if err != nil {
		return err
	}
	if rec == nil {
		return invalid
	}
	if rec.Attempts >= maxCodeAttempts {
		return httpx.NewError(429, "Bạn đã nhập sai mã quá nhiều lần. Vui lòng yêu cầu mã mới.")
	}
	if subtle.ConstantTimeCompare([]byte(rec.TokenHash), []byte(hashCode(u.ID.Hex(), strings.TrimSpace(code)))) != 1 {
		_ = s.resets.Fail(ctx, rec.ID)
		s.audit.Log(nil, u.ID.Hex(), u.Email, "auth.reset_code_invalid", "user", u.ID.Hex(), "", ip)
		return invalid
	}
	if ok, err := s.resets.MarkUsed(ctx, rec.ID); err != nil {
		return err
	} else if !ok {
		return invalid // consumed concurrently
	}
	if err := s.setPassword(ctx, u, password); err != nil {
		return err
	}
	_ = s.users.ClearLock(ctx, u.ID)
	s.audit.Log(nil, u.ID.Hex(), u.Email, "auth.password_reset", "user", u.ID.Hex(), "", ip)
	return nil
}

// ResetNotice is the generic response for the forgot-password endpoint.
func ResetNotice() string { return resetResponse }

// BootstrapAdmin creates the first admin from ADMIN_EMAIL / ADMIN_PASSWORD when no admin exists yet.
func (s *AuthService) BootstrapAdmin(ctx context.Context) error {
	if s.cfg.AdminEmail == "" || s.cfg.AdminPassword == "" {
		return nil
	}
	n, err := s.users.CountAdmins(ctx)
	if err != nil || n > 0 {
		return err
	}
	if _, err := s.CreateUser(ctx, "Administrator", s.cfg.AdminEmail, s.cfg.AdminPassword, domain.RoleAdmin); err != nil {
		return err
	}
	log.Printf("Bootstrap admin created: %s", s.cfg.AdminEmail)
	return nil
}

// IssueTokenForTest signs a token for u without touching the database (used by HTTP tests).
func (s *AuthService) IssueTokenForTest(u *domain.User) (string, error) {
	sess, err := s.session(u)
	if err != nil {
		return "", err
	}
	return sess.Token, nil
}

// SeedUser lets tests pre-populate the auth cache.
func (s *AuthService) SeedUser(u *domain.User) {
	s.mu.Lock()
	s.cache[u.ID.Hex()] = cachedUser{user: u, exp: time.Now().Add(time.Hour)}
	s.mu.Unlock()
}
