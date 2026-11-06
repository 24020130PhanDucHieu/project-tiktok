package handler

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func contextBackground() context.Context { return context.Background() }

// AdminHandler serves /admin routes (admin role only).
type AdminHandler struct {
	admin AdminAPI
	audit AuditReader
	stats DashboardReader
}

// AdminAPI is the part of *service.AdminService this handler uses.
type AdminAPI interface {
	ListUsers(ctx context.Context, q, role, status string, page, limit int64) (*service.UserPage, error)
	CreateUser(ctx context.Context, a service.Actor, name, email, role, password string) (*service.CreatedUser, error)
	UpdateUser(ctx context.Context, a service.Actor, id string, p service.UserPatch) (*domain.User, error)
	SendReset(ctx context.Context, a service.Actor, id string) error
	DeleteUser(ctx context.Context, a service.Actor, id string) error
	System(ctx context.Context) (*service.SystemInfo, error)
}

// AuditReader lists the audit trail (*service.AuditService).
type AuditReader interface {
	List(ctx context.Context, action, q string, page, limit int64) ([]domain.AuditLog, int64, error)
}

// DashboardReader builds the dashboard numbers (*service.StatsService).
type DashboardReader interface {
	Dashboard(ctx context.Context) (*service.Dashboard, error)
}

// NewAdminHandler builds an AdminHandler.
func NewAdminHandler(admin AdminAPI, audit AuditReader, stats DashboardReader) *AdminHandler {
	return &AdminHandler{admin: admin, audit: audit, stats: stats}
}

type createUserRequest struct {
	Name     string `json:"name" validate:"required,min=2,max=120"`
	Email    string `json:"email" validate:"required,email"`
	Role     string `json:"role" validate:"required,oneof=admin member"`
	Password string `json:"password" validate:"omitempty,min=8,max=128"`
}

type updateUserRequest struct {
	Name   *string `json:"name" validate:"omitempty,min=2,max=120"`
	Role   *string `json:"role" validate:"omitempty,oneof=admin member"`
	Status *string `json:"status" validate:"omitempty,oneof=active suspended"`
}

func pageParams(c *fiber.Ctx, defLimit int64) (int64, int64) {
	page, _ := strconv.ParseInt(c.Query("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(c.Query("limit", strconv.FormatInt(defLimit, 10)), 10, 64)
	return page, limit
}

// ListUsers handles GET /admin/users.
func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	page, limit := pageParams(c, 20)
	res, err := h.admin.ListUsers(c.UserContext(), c.Query("q"), c.Query("role"), c.Query("status"), page, limit)
	if err != nil {
		return err
	}
	return httpx.OK(c, res)
}

// CreateUser handles POST /admin/users.
func (h *AdminHandler) CreateUser(c *fiber.Ctx) error {
	var req createUserRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	res, err := h.admin.CreateUser(c.UserContext(), middleware.ActorOf(c), req.Name, req.Email, req.Role, req.Password)
	if err != nil {
		return err
	}
	return httpx.Created(c, res)
}

// UpdateUser handles PATCH /admin/users/:id.
func (h *AdminHandler) UpdateUser(c *fiber.Ctx) error {
	var req updateUserRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	u, err := h.admin.UpdateUser(c.UserContext(), middleware.ActorOf(c), c.Params("id"), service.UserPatch{Name: req.Name, Role: req.Role, Status: req.Status})
	if err != nil {
		return err
	}
	return httpx.OK(c, u)
}

// SendReset handles POST /admin/users/:id/reset-password.
func (h *AdminHandler) SendReset(c *fiber.Ctx) error {
	if err := h.admin.SendReset(c.UserContext(), middleware.ActorOf(c), c.Params("id")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã gửi email đặt lại mật khẩu."})
}

// DeleteUser handles DELETE /admin/users/:id.
func (h *AdminHandler) DeleteUser(c *fiber.Ctx) error {
	if err := h.admin.DeleteUser(c.UserContext(), middleware.ActorOf(c), c.Params("id")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "message": "Đã xóa tài khoản."})
}

// Audit handles GET /admin/audit.
func (h *AdminHandler) Audit(c *fiber.Ctx) error {
	page, limit := pageParams(c, 30)
	page, limit = max(page, 1), min(max(limit, 1), 100)
	items, total, err := h.audit.List(c.UserContext(), c.Query("action"), c.Query("q"), page, limit)
	if err != nil {
		return err
	}
	return httpx.OK(c, fiber.Map{"items": items, "pagination": service.Pagination{Page: page, Limit: limit, Total: total, Pages: (total + limit - 1) / limit}})
}

// System handles GET /admin/system.
func (h *AdminHandler) System(c *fiber.Ctx) error {
	info, err := h.admin.System(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, info)
}
