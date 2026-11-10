// Package handler maps HTTP requests to services.
package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// AuthHandler serves /auth routes.
type AuthHandler struct {
	auth  AuthAPI
	audit Auditor
}

// AuthAPI is the part of *service.AuthService this handler uses.
type AuthAPI interface {
	Register(ctx context.Context, r service.RegistrationRequest) error
	RegistrationOptions() service.RegistrationOptions
	Login(ctx context.Context, email, password, ip string) (*service.Session, error)
	Profile(ctx context.Context, id string) (*domain.User, error)
	UpdateName(ctx context.Context, u *domain.User, name string) error
	ChangePassword(ctx context.Context, u *domain.User, current, next, ip string) (*service.Session, error)
	SendResetCode(ctx context.Context, email, ip string)
	ResetPassword(ctx context.Context, email, code, password, ip string) error
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(auth AuthAPI, audit Auditor) *AuthHandler {
	return &AuthHandler{auth: auth, audit: audit}
}

// registerRequest is the public sign-up form. It has no role or status: whatever a caller adds is ignored, and the
// account is always created as a pending member.
type registerRequest struct {
	Name         string `json:"name" validate:"required,min=2,max=120"`
	Email        string `json:"email" validate:"required,email,max=254"`
	Password     string `json:"password" validate:"required,min=8,max=128"`
	Website      string `json:"website" validate:"max=200"` // honeypot: hidden from people, filled by bots
	CaptchaToken string `json:"captchaToken" validate:"max=4096"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type forgotRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type resetRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Code     string `json:"code" validate:"required,len=6,numeric"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required,max=128"`
	NewPassword     string `json:"newPassword" validate:"required,min=8,max=128"`
}

type profileRequest struct {
	Name string `json:"name" validate:"required,min=2,max=120"`
}

// Register handles POST /auth/register (disabled unless ALLOW_REGISTRATION=true).
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	err := h.auth.Register(c.UserContext(), service.RegistrationRequest{
		Name: req.Name, Email: req.Email, Password: req.Password, Website: req.Website, CaptchaToken: req.CaptchaToken, IP: c.IP(),
	})
	if err != nil {
		return err
	}
	// no token and no account in the answer: the account waits for an admin, and a known email looks the same
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": true, "message": service.RegistrationNotice})
}

// Options handles GET /auth/options (public): whether sign-up is open, and the captcha site key to load.
func (h *AuthHandler) Options(c *fiber.Ctx) error {
	return httpx.OK(c, h.auth.RegistrationOptions())
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	session, err := h.auth.Login(c.UserContext(), req.Email, req.Password, c.IP())
	if err != nil {
		return err
	}
	return httpx.OK(c, session)
}

// Me handles GET /auth/me and returns the current account.
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	u := middleware.CurrentUser(c)
	if u == nil {
		return httpx.Fail(c, fiber.StatusUnauthorized, "Bạn cần đăng nhập")
	}
	return httpx.OK(c, fiber.Map{
		"id": u.ID.Hex(), "sub": u.ID.Hex(), "name": u.Name, "email": u.Email, "role": u.Role,
		"status": u.Status, "lastLoginAt": u.LastLoginAt, "createdAt": u.CreatedAt,
	})
}

// UpdateMe handles PATCH /auth/me.
func (h *AuthHandler) UpdateMe(c *fiber.Ctx) error {
	var req profileRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	u := middleware.CurrentUser(c)
	if err := h.auth.UpdateName(c.UserContext(), u, req.Name); err != nil {
		return err
	}
	u2, err := h.auth.Profile(c.UserContext(), u.ID.Hex())
	if err != nil {
		return err
	}
	return httpx.OK(c, service.ViewOf(u2))
}

// ChangePassword handles POST /auth/change-password (returns a fresh session; old tokens are revoked).
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	var req changePasswordRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	session, err := h.auth.ChangePassword(c.UserContext(), middleware.CurrentUser(c), req.CurrentPassword, req.NewPassword, c.IP())
	if err != nil {
		return err
	}
	return httpx.OKMessage(c, session, "Đã đổi mật khẩu. Các phiên đăng nhập cũ đã bị thu hồi.")
}

// ForgotPassword handles POST /auth/forgot-password (always answers the same message).
func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	var req forgotRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	ip := c.IP()
	// Work is done in the background so response time cannot reveal whether the account exists.
	go h.auth.SendResetCode(contextBackground(), req.Email, ip)
	return c.JSON(fiber.Map{"success": true, "message": service.ResetNotice()})
}

// ResetPassword handles POST /auth/reset-password.
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req resetRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.auth.ResetPassword(c.UserContext(), req.Email, req.Code, req.Password, c.IP()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã đặt lại mật khẩu. Bạn có thể đăng nhập bằng mật khẩu mới."})
}
