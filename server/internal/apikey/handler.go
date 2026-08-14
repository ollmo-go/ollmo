package apikey

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type CreateInput struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Create issues a new API key. The full key string is returned in the
// response body and is never persisted or retrievable again.
func (h *Handler) Create(c *fiber.Ctx) error {
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	k, fullKey, err := h.svc.Create(middleware.TenantID(c), middleware.UserID(c), in.Name, in.ExpiresAt)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, fiber.Map{
		"key":      k,
		"full_key": fullKey,
	})
}

func (h *Handler) List(c *fiber.Ctx) error {
	items, err := h.svc.List(middleware.TenantID(c), middleware.UserID(c), middleware.Role(c) == "admin")
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": items})
}

func (h *Handler) Revoke(c *fiber.Ctx) error {
	if err := h.svc.Revoke(middleware.TenantID(c), middleware.UserID(c), c.Params("id"), middleware.Role(c) == "admin"); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "revoked"})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(middleware.TenantID(c), middleware.UserID(c), c.Params("id"), middleware.Role(c) == "admin"); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}
