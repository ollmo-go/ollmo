package provider

import (
	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

// Handler serves one kind's provider routes. Three instances exist
// (chat/embedding/rerank), each mounted under its model route group.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *fiber.Ctx) error {
	cards, err := h.svc.List(middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": cards})
}

func (h *Handler) Catalog(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{"items": h.svc.Catalog()})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	card, err := h.svc.Create(middleware.TenantID(c), middleware.UserID(c), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, card)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	var in UpdateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	card, err := h.svc.Update(middleware.TenantID(c), c.Params("id"), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, card)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(middleware.TenantID(c), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"deleted": true})
}

func (h *Handler) Discover(c *fiber.Ctx) error {
	models, err := h.svc.Discover(c.Context(), middleware.TenantID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": models})
}

// Probe asks an arbitrary endpoint+key (form state) for its model list,
// before any provider card exists or is saved.
func (h *Handler) Probe(c *fiber.Ctx) error {
	var in ProbeInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	models, err := h.svc.Probe(c.Context(), middleware.TenantID(c), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": models})
}

func (h *Handler) AddModel(c *fiber.Ctx) error {
	var in AddModelInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	kind := c.Params("kind")
	ref, err := h.svc.AddModel(middleware.TenantID(c), middleware.UserID(c), c.Params("id"), kind, in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, ref)
}

func (h *Handler) UpdateModel(c *fiber.Ctx) error {
	var in UpdateModelInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	kind := c.Params("kind")
	ref, err := h.svc.UpdateModel(middleware.TenantID(c), c.Params("id"), c.Params("mid"), kind, in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, ref)
}

func (h *Handler) RemoveModel(c *fiber.Ctx) error {
	kind := c.Params("kind")
	if err := h.svc.RemoveModel(middleware.TenantID(c), c.Params("id"), c.Params("mid"), kind); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"deleted": true})
}
