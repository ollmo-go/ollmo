package agent

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

// Get returns the agent definition for a KB. If no agent exists yet, a default
// one is seeded so the canvas always opens with a valid graph.
func (h *Handler) Get(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	a, err := h.svc.Get(c.Context(), tenantID, kbID)
	if err != nil {
		return response.Fail(c, err)
	}
	var def Definition
	if err := json.Unmarshal([]byte(a.Definition), &def); err != nil {
		log.Printf("[agent] unmarshal definition failed kb=%s: %v", kbID, err)
	}
	migrateNodeTypes(&def)
	return response.OK(c, fiber.Map{
		"id":         a.ID,
		"kb_id":      a.KbID,
		"name":       a.Name,
		"version":    a.Version,
		"definition": def,
		"active":     a.Active,
		"updated_at": a.UpdatedAt,
	})
}

// Save replaces the agent definition. The body is the React Flow graph.
func (h *Handler) Save(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	var def Definition
	if err := c.BodyParser(&def); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	migrateNodeTypes(&def)
	a, err := h.svc.Save(c.Context(), tenantID, kbID, def)
	if err != nil {
		return response.Fail(c, err)
	}
	var out Definition
	if err := json.Unmarshal([]byte(a.Definition), &out); err != nil {
		log.Printf("[agent] unmarshal definition after save failed kb=%s: %v", kbID, err)
	}
	return response.OK(c, fiber.Map{
		"id":         a.ID,
		"kb_id":      a.KbID,
		"name":       a.Name,
		"version":    a.Version,
		"definition": out,
		"active":     a.Active,
		"updated_at": a.UpdatedAt,
	})
}

// migrateNodeTypes converts legacy node type names ("input"/"output") to the
// current constants ("start"/"end"). This keeps existing saved agents working
// after the rename to avoid clashing with ReactFlow's built-in node types.
func migrateNodeTypes(def *Definition) {
	for i := range def.Nodes {
		switch def.Nodes[i].Type {
		case "input":
			def.Nodes[i].Type = NodeInput
		case "output":
			def.Nodes[i].Type = NodeOutput
		}
	}
}
