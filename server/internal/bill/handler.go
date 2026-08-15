package bill

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Overview returns the tenant's total tokens and estimated cost.
func (h *Handler) Overview(c *fiber.Ctx) error {
	o, err := h.svc.Overview(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, o)
}

// Users returns per-user token/cost totals, ranked by total tokens.
func (h *Handler) Users(c *fiber.Ctx) error {
	items, err := h.svc.UserTotals(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}

// Models returns per-model token/cost totals, ranked by total tokens.
func (h *Handler) Models(c *fiber.Ctx) error {
	items, err := h.svc.ModelTotals(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}

// Records returns the most recent raw bill rows (newest first).
func (h *Handler) Records(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	items, err := h.svc.List(c.Context(), middleware.TenantID(c), limit)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}
