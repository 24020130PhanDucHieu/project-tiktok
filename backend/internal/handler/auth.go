// Package handler maps HTTP requests to services.
package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// AuthHandler serves /auth routes.
type AuthHandler struct{ auth *service.AuthService }

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(auth *service.AuthService) *AuthHandler { return &AuthHandler{auth: auth} }

type registerRequest struct {
	Name     string `json:"name" validate:"required,min=2,max=120"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// Register handles POST /auth/register.
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
	session, err := h.auth.Login(c.UserContext(), req.Email, req.Password)
	if err != nil {
		return err
	}
	return httpx.OK(c, session)
}

// Me handles GET /auth/me and returns the decoded token payload.
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	claims, _ := c.Locals(middleware.UserKey).(*service.Claims)
	if claims == nil {
		return httpx.Fail(c, fiber.StatusUnauthorized, "Bạn cần đăng nhập")
	}
	return httpx.OK(c, fiber.Map{
		"sub": claims.Subject, "role": claims.Role, "name": claims.Name, "email": claims.Email,
		"iat": claims.IssuedAt.Unix(), "exp": claims.ExpiresAt.Unix(),
	})
}
