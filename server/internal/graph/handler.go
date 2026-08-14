package graph

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

// List returns the top entities in a KB, sorted by mention count. This feeds
// the "Knowledge Graph" panel in the KB detail page.
func (h *Handler) List(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "50"))
	items, total, err := h.svc.ListEntities(tenantID, kbID, page, size)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
}
