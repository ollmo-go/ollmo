package pipeline

import (
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Get returns the pipeline definition for a KB. If no pipeline exists yet, a
// default one is seeded from the KB config so the canvas always opens with a
// valid graph.
func (h *Handler) Get(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	p, err := h.svc.Get(c.Context(), tenantID, kbID)
	if err != nil {
		return response.Fail(c, err)
	}
	var def Definition
	if err := json.Unmarshal([]byte(p.Definition), &def); err != nil {
		log.Printf("[pipeline] unmarshal definition failed kb=%s: %v", kbID, err)
	}
	return response.OK(c, fiber.Map{
		"id":         p.ID,
		"kb_id":      p.KbID,
		"version":    p.Version,
		"definition": def,
		"active":     p.Active,
		"updated_at": p.UpdatedAt,
	})
}

// Save replaces the pipeline definition. The body is the React Flow graph.
func (h *Handler) Save(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	var def Definition
	if err := c.BodyParser(&def); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	p, err := h.svc.Save(c.Context(), tenantID, kbID, def)
	if err != nil {
		return response.Fail(c, err)
	}
	var out Definition
	if err := json.Unmarshal([]byte(p.Definition), &out); err != nil {
		log.Printf("[pipeline] unmarshal definition after save failed kb=%s: %v", kbID, err)
	}
	return response.OK(c, fiber.Map{
		"id":         p.ID,
		"kb_id":      p.KbID,
		"version":    p.Version,
		"definition": out,
		"active":     p.Active,
		"updated_at": p.UpdatedAt,
	})
}
