package install

import (
	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Status — GET /install/status (public)
func (h *Handler) Status(c *fiber.Ctx) error {
	st, err := h.svc.Status(c.Context())
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, st)
}

// Defaults — GET /install/defaults (public)
func (h *Handler) Defaults(c *fiber.Ctx) error {
	return response.OK(c, Defaults())
}

// Install — POST /install (public, only works when no users exist)
func (h *Handler) Install(c *fiber.Ctx) error {
	var in InstallInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	token, err := h.svc.Install(c.Context(), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, token)
}
