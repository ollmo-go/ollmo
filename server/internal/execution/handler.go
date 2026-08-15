package execution

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	repo *Repo
}

func NewHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

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
	return response.OK(c, e)
}

// GetByMessage returns the execution that produced one assistant message.
// Lets the analytics feedback list deep-link a bad case straight into replay.
func (h *Handler) GetByMessage(c *fiber.Ctx) error {
	e, err := h.repo.FindByMessageID(middleware.TenantID(c), c.Params("messageId"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, e)
}
