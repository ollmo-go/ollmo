package audit

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
)

// Middleware records audit logs for write operations (POST/PUT/DELETE).
// It runs after the auth middleware so user/tenant context is available.
// Only successful (2xx) writes are recorded. The action is derived from the
// HTTP method and the first path segment after /api/v1.
func Middleware(svc *Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()

		method := c.Method()
		if method != "POST" && method != "PUT" && method != "DELETE" {
			return err
		}
		if c.Response().StatusCode() < 200 || c.Response().StatusCode() >= 300 {
			return err
		}

		tenantID := middleware.TenantID(c)
		userID := middleware.UserID(c)
		if tenantID == "" || userID == "" {
			return err
		}

		path := strings.TrimPrefix(c.Path(), "/api/v1/")
		segments := strings.Split(path, "/")
		resource := ""
		action := strings.ToLower(method)
		if len(segments) > 0 {
			resource = segments[0]
			action = resource + "." + strings.ToLower(method)
		}
		// Capture the last id-like segment as the resource id.
		resID := ""
		for i := len(segments) - 1; i >= 0; i-- {
			s := segments[i]
			if s != "" && s != "stream" {
				resID = s
				break
			}
		}

		svc.Record(c.Context(), tenantID, userID, "", action, resID, c.Path(), c.IP())
		return err
	}
}
