package annotation

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "50"))
	items, total, err := h.svc.List(c.Context(), middleware.TenantID(c), c.Params("kbId"), page, size)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items, "total": total, "page": page, "size": size})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	a, err := h.svc.Create(c.Context(), middleware.TenantID(c), c.Params("kbId"), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, a)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	var in UpdateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	a, err := h.svc.Update(c.Context(), middleware.TenantID(c), c.Params("kbId"), c.Params("id"), in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, a)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(c.Context(), middleware.TenantID(c), c.Params("kbId"), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}
