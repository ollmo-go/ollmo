package search

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

func (h *Handler) Search(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	var req SearchRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	out, err := h.svc.Search(c.Context(), middleware.TenantID(c), kbID, req)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, out)
}

// Debug runs a search with debug instrumentation enabled and returns the
// hits plus the intermediate dense/sparse hits and RRF scores so callers
// can understand why each chunk was selected.
func (h *Handler) Debug(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	var req SearchRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	out, err := h.svc.SearchDebug(c.Context(), middleware.TenantID(c), kbID, req)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, out)
}
