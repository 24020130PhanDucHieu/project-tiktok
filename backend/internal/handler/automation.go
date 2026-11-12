package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/middleware"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// AutomationStore is the master switch for what the system sends to creators on its own
// (*repository.AutomationRepository).
type AutomationStore interface {
	State(ctx context.Context) (repository.AutomationState, error)
	SetEnabled(ctx context.Context, enabled bool, by string) error
}

// AutomationHandler serves the switch. Reading is open to every signed-in member (the chat shows it); changing it
// reaches creators, so it is admin-only (router) and needs a confirmation to turn on.
type AutomationHandler struct {
	store AutomationStore
	audit Auditor
}

// NewAutomationHandler builds the handler.
func NewAutomationHandler(store AutomationStore, audit Auditor) *AutomationHandler {
	return &AutomationHandler{store: store, audit: audit}
}

// Get handles GET /automation. A switch that cannot be read is reported as off.
func (h *AutomationHandler) Get(c *fiber.Ctx) error {
	st, err := h.store.State(c.UserContext())
	if err != nil {
		st.Enabled = false
	}
	return httpx.OK(c, fiber.Map{"enabled": st.Enabled, "updatedBy": st.UpdatedBy, "updatedAt": st.UpdatedAt})
}

// Set handles PUT /automation with {enabled, confirm}.
func (h *AutomationHandler) Set(c *fiber.Ctx) error {
	var in struct {
		Enabled bool `json:"enabled"`
		Confirm bool `json:"confirm"`
	}
	if err := c.BodyParser(&in); err != nil {
		return httpx.Fail(c, fiber.StatusBadRequest, "Dữ liệu không hợp lệ")
	}
	if in.Enabled && !in.Confirm {
		return httpx.Fail(c, fiber.StatusBadRequest, "Cần xác nhận trước khi bật gửi tin tự động.")
	}
	a := middleware.ActorOf(c)
	if err := h.store.SetEnabled(c.UserContext(), in.Enabled, a.Email); err != nil {
		return httpx.Fail(c, fiber.StatusInternalServerError, "Không lưu được công tắc, trạng thái giữ nguyên.")
	}
	action := "automation.switch_off"
	if in.Enabled {
		action = "automation.switch_on"
	}
	h.audit.Log(c.UserContext(), a.ID, a.Email, action, "automation", "", "", a.IP)
	return httpx.OK(c, fiber.Map{"enabled": in.Enabled})
}
