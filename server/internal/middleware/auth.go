package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Claims is the JWT payload. TenantID is enforced on every authenticated
// request to keep multi-tenant isolation tight.
type Claims struct {
	UserID       string `json:"user_id"`
	TenantID     string `json:"tenant_id"`
	Role         string `json:"role"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	jwt.RegisteredClaims
}

// IssueToken signs a JWT for the given user.
func IssueToken(secret string, userID, tenantID, role string, isSuperAdmin bool, expireHours int) (string, error) {
	claims := Claims{
		UserID:       userID,
		TenantID:     tenantID,
		Role:         role,
		IsSuperAdmin: isSuperAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// TokenRevoker checks and records revoked JWT token IDs (jti). When wired
// (via Redis), logout adds the token's jti to the revocation set so the token
// can't be reused even before it expires.
type TokenRevoker interface {
	IsRevoked(ctx context.Context, jti string) bool
	Revoke(ctx context.Context, jti string, ttl time.Duration) error
}

// RedisTokenRevoker implements TokenRevoker using Redis with a TTL equal to the
// token's remaining lifetime, so revoked entries auto-expire when the JWT
// would have expired anyway.
type RedisTokenRevoker struct{ rdb *redis.Client }

func NewRedisTokenRevoker(rdb *redis.Client) *RedisTokenRevoker {
	return &RedisTokenRevoker{rdb: rdb}
}

func (r *RedisTokenRevoker) IsRevoked(ctx context.Context, jti string) bool {
	if jti == "" || r.rdb == nil {
		return false
	}
	n, err := r.rdb.Exists(ctx, "jwt:revoked:"+jti).Result()
	return err == nil && n > 0
}

func (r *RedisTokenRevoker) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" || r.rdb == nil {
		return nil
	}
	return r.rdb.Set(ctx, "jwt:revoked:"+jti, "1", ttl).Err()
}

// Auth middleware parses the JWT from either the Authorization: Bearer header
// or the auth_token httpOnly cookie, and stores the claims into Locals under
// "user_id", "tenant_id", "role". The cookie takes priority for browser
// requests; the header is kept for API clients and SSE streams. When a
// TokenRevoker is provided, revoked tokens (logout) are rejected.
func Auth(secret string, revoker TokenRevoker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenStr := ""

		// 1. Try httpOnly cookie (set by login/register).
		if cookie := c.Cookies("auth_token"); cookie != "" {
			tokenStr = cookie
		}

		// 2. Fall back to Authorization header (API clients, SSE).
		if tokenStr == "" {
			authHeader := c.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if tokenStr == "" {
			return c.Status(401).JSON(fiber.Map{"code": 2, "message": "missing or invalid Authorization header"})
		}

		claims := &Claims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"code": 2, "message": "invalid token: " + err.Error()})
		}
		if revoker != nil && claims.ID != "" && revoker.IsRevoked(c.Context(), claims.ID) {
			return c.Status(401).JSON(fiber.Map{"code": 2, "message": "token revoked"})
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("tenant_id", claims.TenantID)
		c.Locals("role", claims.Role)
		c.Locals("is_super_admin", claims.IsSuperAdmin)
		return c.Next()
	}
}

// Context helpers used by handlers and GORM scopes.

func UserID(c *fiber.Ctx) string {
	if v, ok := c.Locals("user_id").(string); ok {
		return v
	}
	return ""
}

func TenantID(c *fiber.Ctx) string {
	if v, ok := c.Locals("tenant_id").(string); ok {
		return v
	}
	return ""
}

func Role(c *fiber.Ctx) string {
	if v, ok := c.Locals("role").(string); ok {
		return v
	}
	return ""
}

func IsSuperAdmin(c *fiber.Ctx) bool {
	if v, ok := c.Locals("is_super_admin").(bool); ok {
		return v
	}
	return false
}

// AdminOnly blocks non-admin users from the route. Super admins are allowed.
// Must be used after Auth.
func AdminOnly() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if Role(c) != "admin" && !IsSuperAdmin(c) {
			return c.Status(403).JSON(fiber.Map{"code": 3, "message": "admin permission required"})
		}
		return c.Next()
	}
}

// SuperAdminOnly blocks non-super-admin users. Must be used after Auth.
func SuperAdminOnly() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !IsSuperAdmin(c) {
			return c.Status(403).JSON(fiber.Map{"code": 3, "message": "super admin permission required"})
		}
		return c.Next()
	}
}
