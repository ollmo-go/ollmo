package apikey

import "github.com/gofiber/fiber/v2"

// Middleware authenticates requests using the X-API-Key header. On success
// it stores tenant_id and user_id in Locals, mirroring the JWT middleware so
// downstream handlers and GORM scopes work unchanged.
func Middleware(repo *Repo) fiber.Handler {
	svc := NewService(repo)
	return func(c *fiber.Ctx) error {
		raw := c.Get("X-API-Key")
		if raw == "" {
			return c.Status(401).JSON(fiber.Map{"code": 2, "message": "missing X-API-Key header"})
		}
		key, err := svc.Verify(raw)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"code": 2, "message": "invalid api key"})
		}
		c.Locals("tenant_id", key.TenantID)
		c.Locals("user_id", key.UserID)
		return c.Next()
	}
}

// TenantID reads the tenant id set by the API key middleware from Locals.
func TenantID(c *fiber.Ctx) string {
	if v, ok := c.Locals("tenant_id").(string); ok {
		return v
	}
	return ""
}

// UserID reads the user id set by the API key middleware from Locals.
func UserID(c *fiber.Ctx) string {
	if v, ok := c.Locals("user_id").(string); ok {
		return v
	}
	return ""
}
