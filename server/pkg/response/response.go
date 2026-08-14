package response

import (
	"github.com/gofiber/fiber/v2"
	"ollmo/ollmo/pkg/errs"
)

// Response is the unified envelope for all API responses.
type Response struct {
	Code    errs.Code `json:"code"`
	Message string    `json:"message"`
	Data    any       `json:"data,omitempty"`
	TraceID string    `json:"trace_id,omitempty"`
}

func OK(c *fiber.Ctx, data any) error {
	return c.JSON(Response{Code: errs.CodeOK, Message: "ok", Data: data})
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(201).JSON(Response{Code: errs.CodeOK, Message: "created", Data: data})
}

func NoContent(c *fiber.Ctx) error {
	return c.Status(204).Send(nil)
}

// Fail returns an error response. Status code is derived from errs.Code.
func Fail(c *fiber.Ctx, err error) error {
	if e, ok := err.(*errs.Error); ok {
		return c.Status(e.HTTPStatus()).JSON(Response{Code: e.Code, Message: e.Error()})
	}
	return c.Status(500).JSON(Response{Code: errs.CodeInternal, Message: err.Error()})
}
