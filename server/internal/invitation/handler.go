package invitation

import (
	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Create — POST /tenant/invitations (admin only)
func (h *Handler) Create(c *fiber.Ctx) error {
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	view, token, err := h.svc.Create(c.Context(), middleware.TenantID(c), middleware.UserID(c), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, fiber.Map{
		"invitation": view,
		"token":      token,
	})
}

// List — GET /tenant/invitations (admin only)
func (h *Handler) List(c *fiber.Ctx) error {
	views, err := h.svc.List(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": views})
}

// Cancel — DELETE /tenant/invitations/:id (admin only)
func (h *Handler) Cancel(c *fiber.Ctx) error {
	if err := h.svc.Cancel(c.Context(), middleware.TenantID(c), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}

// Peek — GET /invitations/:token (public)
func (h *Handler) Peek(c *fiber.Ctx) error {
	view, err := h.svc.Peek(c.Context(), c.Params("token"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, view)
}

// Accept — POST /invitations/accept (public)
func (h *Handler) Accept(c *fiber.Ctx) error {
	var in AcceptInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	u, err := h.svc.Accept(c.Context(), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"user_id":   u.ID,
		"tenant_id": u.TenantID,
		"email":     u.Email,
	})
}
