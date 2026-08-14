package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// TenantRateLimit returns a middleware that limits requests per tenant using a
// Redis fixed-window counter. Each tenant gets `max` requests per `window`.
// When the limit is exceeded the middleware responds 429 with a Retry-After
// header. Requests without a tenant (unauthenticated) are not rate-limited
// here; the global per-IP limiter covers those.
func TenantRateLimit(rdb *redis.Client, max int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := TenantID(c)
		if tenantID == "" || rdb == nil {
			return c.Next()
		}

		key := fmt.Sprintf("ratelimit:tenant:%s:%d", tenantID, time.Now().Unix()/int64(window.Seconds()))
		ctx := c.Context()
		n, err := rdb.Incr(ctx, key).Result()
		if err == nil && n == 1 {
			rdb.Expire(ctx, key, window)
		}
		if n > int64(max) {
			c.Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			return c.Status(429).JSON(fiber.Map{
				"code":    5,
				"message": "rate limit exceeded for your tenant",
			})
		}
		return c.Next()
	}
}
