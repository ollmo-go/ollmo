package site

import (
	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GetPublic returns non-sensitive settings (site name, language, etc.)
// accessible without authentication. The frontend uses this to render the
// site name and decide whether to show the registration link.
func (h *Handler) GetPublic(c *fiber.Ctx) error {
	all := h.svc.All()
	public := make(fiber.Map, len(PublicKeys))
	for _, k := range PublicKeys {
		public[k] = all[k]
	}
	return response.OK(c, public)
}

// GetAll returns all settings. Super admin only.
func (h *Handler) GetAll(c *fiber.Ctx) error {
	return response.OK(c, h.svc.All())
}

// Update accepts a JSON object of key-value pairs and updates them.
// Super admin only.
func (h *Handler) Update(c *fiber.Ctx) error {
	var in map[string]string
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if len(in) == 0 {
		return response.Fail(c, errs.BadRequest("no settings to update"))
	}
	if err := h.svc.Update(in); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}
