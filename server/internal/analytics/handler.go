package analytics

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

func (h *Handler) Overview(c *fiber.Ctx) error {
	o, err := h.svc.Overview(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, o)
}

func (h *Handler) DocStats(c *fiber.Ctx) error {
	s, err := h.svc.DocStats(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, s)
}

func (h *Handler) KBUsage(c *fiber.Ctx) error {
	items, err := h.svc.KBUsage(c.Context(), middleware.TenantID(c))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}

func (h *Handler) RecentActivity(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	items, err := h.svc.RecentActivity(c.Context(), middleware.TenantID(c), limit)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}
