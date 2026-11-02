package handler

import (
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

func (h *InboxHandler) contactKoc(c *fiber.Ctx) (*domain.Koc, error) {
	koc, err := h.kocs.FindActiveByID(c.UserContext(), c.Params("id"))
	if err != nil {
		return nil, err
	}
	if koc == nil {
		return nil, httpx.NewError(fiber.StatusNotFound, "Không tìm thấy KOC đang hoạt động")
	}
	return koc, nil
}

func hasContact(list []domain.Contact, typ, value string) bool {
	for _, ct := range list {
		if ct.Type == typ && ct.Value == value {
			return true
		}
	}
	return false
}

// ListContacts handles GET /kocs/:id/contacts. The KOC's own phone and email are folded in once.
func (h *InboxHandler) ListContacts(c *fiber.Ctx) error {
	koc, err := h.contactKoc(c)
	if err != nil {
		return err
	}
	changed := false
	if p := service.NormalizePhone(koc.Phone); p != "" && !hasContact(koc.Contacts, domain.ContactPhone, p) {
		koc.Contacts = append(koc.Contacts, domain.Contact{ID: primitive.NewObjectID().Hex(), Type: domain.ContactPhone, Value: p, Source: "Danh bạ KOC", UpdatedAt: koc.UpdatedAt})
		changed = true
	}
	if e := strings.ToLower(strings.TrimSpace(koc.Email)); e != "" && !hasContact(koc.Contacts, domain.ContactEmail, e) {
		koc.Contacts = append(koc.Contacts, domain.Contact{ID: primitive.NewObjectID().Hex(), Type: domain.ContactEmail, Value: e, Source: "Danh bạ KOC", UpdatedAt: koc.UpdatedAt})
		changed = true
	}
	if changed {
		if err := h.kocs.Save(c.UserContext(), koc); err != nil {
			return err
		}
	}
	if koc.Contacts == nil {
		koc.Contacts = []domain.Contact{}
	}
	return httpx.OK(c, koc.Contacts)
}

// AddContact handles POST /kocs/:id/contacts.
func (h *InboxHandler) AddContact(c *fiber.Ctx) error {
	var req struct {
		Type  string `json:"type" validate:"required,oneof=phone email"`
		Value string `json:"value" validate:"required,max=120"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	koc, err := h.contactKoc(c)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(req.Value)
	if req.Type == domain.ContactPhone {
		if value = service.NormalizePhone(value); len(value) < 9 {
			return httpx.NewError(fiber.StatusBadRequest, "Số điện thoại không hợp lệ")
		}
	} else {
		a, err := mail.ParseAddress(value)
		if err != nil {
			return httpx.NewError(fiber.StatusBadRequest, "Địa chỉ email không hợp lệ")
		}
		value = strings.ToLower(a.Address)
	}
	if hasContact(koc.Contacts, req.Type, value) {
		return httpx.NewError(fiber.StatusConflict, "Liên hệ này đã có trong danh sách")
	}
	ct := domain.Contact{ID: primitive.NewObjectID().Hex(), Type: req.Type, Value: value, Source: "Bạn thêm", UpdatedAt: time.Now()}
	koc.Contacts = append(koc.Contacts, ct)
	if req.Type == domain.ContactPhone && koc.Phone == "" {
		koc.Phone = value
	}
	if req.Type == domain.ContactEmail && koc.Email == "" {
		koc.Email = value
	}
	if err := h.kocs.Save(c.UserContext(), koc); err != nil {
		return err
	}
	return httpx.Created(c, ct)
}

// PatchContact handles PATCH /kocs/:id/contacts/:cid (mark or clear "không tới được").
func (h *InboxHandler) PatchContact(c *fiber.Ctx) error {
	var req struct {
		Unreachable bool `json:"unreachable"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	koc, err := h.contactKoc(c)
	if err != nil {
		return err
	}
	for i := range koc.Contacts {
		if koc.Contacts[i].ID == c.Params("cid") {
			koc.Contacts[i].Unreachable, koc.Contacts[i].UpdatedAt = req.Unreachable, time.Now()
			if err := h.kocs.Save(c.UserContext(), koc); err != nil {
				return err
			}
			return httpx.OK(c, koc.Contacts[i])
		}
	}
	return httpx.NewError(fiber.StatusNotFound, "Không tìm thấy liên hệ")
}

// ListTemplates handles GET /inbox/templates.
func (h *InboxHandler) ListTemplates(c *fiber.Ctx) error {
	items, err := h.convs.ListTemplates(c.UserContext())
	if err != nil {
		return err
	}
	return httpx.OK(c, items)
}

// CreateTemplate handles POST /inbox/templates.
func (h *InboxHandler) CreateTemplate(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name" validate:"required,min=1,max=60"`
		Body string `json:"body" validate:"required,min=1,max=4000"`
	}
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}
	t := &domain.MessageTemplate{Name: strings.TrimSpace(req.Name), Body: req.Body}
	if err := h.convs.CreateTemplate(c.UserContext(), t); err != nil {
		return err
	}
	return httpx.Created(c, t)
}

// DeleteTemplate handles DELETE /inbox/templates/:templateId.
func (h *InboxHandler) DeleteTemplate(c *fiber.Ctx) error {
	if err := h.convs.DeleteTemplate(c.UserContext(), c.Params("templateId")); err != nil {
		return err
	}
	return httpx.OKMessage(c, nil, "Đã xóa mẫu.")
}
