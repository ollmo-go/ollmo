package memory

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

// Summarize generates and stores a conversation summary. The caller must
// own the conversation.
func (h *Handler) Summarize(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	userID := middleware.UserID(c)
	convID := c.Params("id")
	m, err := h.svc.SummarizeConversation(c.Context(), tenantID, userID, convID)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, m)
}

// List returns the caller's memory summaries for a KB. Memories are scoped
// per user so shared-KB members only see their own.
func (h *Handler) List(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	userID := middleware.UserID(c)
	kbID := c.Params("kbId")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "20"))
	items, total, err := h.svc.List(tenantID, userID, kbID, page, size)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"items": items, "total": total, "page": page, "size": size,
	})
}

// Delete removes a memory summary. The kbId path param is enforced by the
// kbOwner middleware on the route.
func (h *Handler) Delete(c *fiber.Ctx) error {
	tenantID := middleware.TenantID(c)
	kbID := c.Params("kbId")
	id := c.Params("id")
	if err := h.svc.Delete(tenantID, kbID, id); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}
