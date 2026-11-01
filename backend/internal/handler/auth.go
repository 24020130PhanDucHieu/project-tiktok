// Package handler maps HTTP requests to services.
package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// AuthHandler serves /auth routes.
type AuthHandler struct {
	auth  *service.AuthService
	audit *service.AuditService
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(auth *service.AuthService, audit *service.AuditService) *AuthHandler {
	return &AuthHandler{auth: auth, audit: audit}
}

type registerRequest struct {
	Name     string `json:"name" validate:"required,min=2,max=120"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type forgotRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type resetRequest struct {
	Token    string `json:"token" validate:"required,min=20,max=200"`
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
	session, err := h.auth.Register(c.UserContext(), req.Name, req.Email, req.Password)
	if err != nil {
		return err
	}
	return httpx.Created(c, session)
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
	go h.auth.SendResetLink(contextBackground(), req.Email, ip)
	return c.JSON(fiber.Map{"success": true, "message": service.ResetNotice()})
}

// ResetPassword handles POST /auth/reset-password.
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req resetRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.auth.ResetPassword(c.UserContext(), req.Token, req.Password, c.IP()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã đặt lại mật khẩu. Bạn có thể đăng nhập bằng mật khẩu mới."})
}
