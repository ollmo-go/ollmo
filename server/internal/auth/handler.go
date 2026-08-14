package auth

import (
	"log"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc       *Service
	jwtExpire int
}

func NewHandler(svc *Service, jwtExpireHours int) *Handler {
	return &Handler{svc: svc, jwtExpire: jwtExpireHours}
}

// setAuthCookie sets the JWT as an httpOnly cookie for XSS protection.
// The token is also returned in the JSON body so the frontend can read
// the expiry and user info; the cookie is the authoritative auth source.
func setAuthCookie(c *fiber.Ctx, token string, expireHours int) {
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    token,
		HTTPOnly: true,
		Secure:   false, // set true in production behind TLS
		SameSite: "Lax",
		MaxAge:   expireHours * 3600,
		Path:     "/",
	})
}

func (h *Handler) Register(c *fiber.Ctx) error {
	var in RegisterInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	tok, err := h.svc.Register(c.Context(), in)
	if err != nil {
		return response.Fail(c, err)
	}
	setAuthCookie(c, tok.Token, h.jwtExpire)
	return response.Created(c, tok)
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var in LoginInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	tok, err := h.svc.Login(c.Context(), in)
	if err != nil {
		return response.Fail(c, err)
	}
	setAuthCookie(c, tok.Token, h.jwtExpire)
	return response.OK(c, tok)
}

// Me returns the current authenticated user identity from the JWT.
func (h *Handler) Me(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{
		"user_id":       middleware.UserID(c),
		"tenant_id":     middleware.TenantID(c),
		"role":          middleware.Role(c),
		"is_super_admin": middleware.IsSuperAdmin(c),
	})
}

// Logout revokes the JWT (adds its jti to the Redis revocation set) and
// clears the auth cookie. After this, the token is rejected by the Auth
// middleware even before its natural expiry.
func (h *Handler) Logout(c *fiber.Ctx) error {
	if token := c.Cookies("auth_token"); token != "" {
		if err := h.svc.Logout(c.Context(), token); err != nil {
			log.Printf("[auth] revoke token failed: %v", err)
		}
	}
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    "",
		HTTPOnly: true,
		MaxAge:   -1,
		Path:     "/",
	})
	return c.SendStatus(204)
}
