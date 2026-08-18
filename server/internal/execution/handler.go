package execution

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

// UserResolver batch-resolves user IDs to display names. Returning an
// empty map is valid — the affected executions simply won't have user_name.
type UserResolver func(tenantID string, userIDs []string) map[string]string

type Handler struct {
	repo   *Repo
	users  UserResolver
}

func NewHandler(repo *Repo, users UserResolver) *Handler {
	return &Handler{repo: repo, users: users}
}

func (h *Handler) enrichUsers(tenantID string, items []*Execution) {
	if h.users == nil || len(items) == 0 {
		return
	}
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(items))
	for _, e := range items {
		if e.UserID != "" {
			if _, ok := seen[e.UserID]; !ok {
				seen[e.UserID] = struct{}{}
				ids = append(ids, e.UserID)
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	names := h.users(tenantID, ids)
	for _, e := range items {
		if name, ok := names[e.UserID]; ok {
			e.UserName = name
		}
	}
}

func (h *Handler) enrichUser(tenantID string, e *Execution) {
	if h.users == nil || e.UserID == "" {
		return
	}
	names := h.users(tenantID, []string{e.UserID})
	if name, ok := names[e.UserID]; ok {
		e.UserName = name
	}
}

// List returns executions for one KB, newest first. Query params: page, size,
// source (chat|test; empty returns both).
func (h *Handler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	items, total, err := h.repo.List(middleware.TenantID(c), c.Params("kbId"), c.Query("source"), page, size)
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "list executions", err))
	}
	h.enrichUsers(middleware.TenantID(c), items)
	return response.OK(c, fiber.Map{
		"items": items, "total": total, "page": page, "size": size,
	})
}

// Get returns one execution with its full trace for replay.
func (h *Handler) Get(c *fiber.Ctx) error {
	e, err := h.repo.Find(middleware.TenantID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	h.enrichUser(middleware.TenantID(c), e)
	return response.OK(c, e)
}

// GetByMessage returns the execution that produced one assistant message.
// Lets the analytics feedback list deep-link a bad case straight into replay.
func (h *Handler) GetByMessage(c *fiber.Ctx) error {
	e, err := h.repo.FindByMessageID(middleware.TenantID(c), c.Params("messageId"))
	if err != nil {
		return response.Fail(c, err)
	}
	h.enrichUser(middleware.TenantID(c), e)
	return response.OK(c, e)
}
