package user

import (
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	repo       *Repo
	tenantRepo *tenant.Repo
	// messageUsed returns today's message counts keyed by user ID. Optional;
	// when nil the member list omits usage. Injected by the server package
	// to keep user decoupled from Redis.
	messageUsed func(userIDs []string) map[string]int
}

func NewHandler(repo *Repo, tenantRepo *tenant.Repo) *Handler {
	return &Handler{repo: repo, tenantRepo: tenantRepo}
}

// WithMessageUsage injects the per-user daily message usage lookup.
func (h *Handler) WithMessageUsage(f func(userIDs []string) map[string]int) *Handler {
	h.messageUsed = f
	return h
}

type UserView struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Language     string `json:"language"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	TenantName   string `json:"tenant_name"`
	CreatedAt    string `json:"created_at"`
	MessageUsed  int    `json:"message_used"`
}

func toView(u *User, tenantName string) *UserView {
	return &UserView{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		Language:     u.Language,
		Role:         u.Role,
		Status:       u.Status,
		IsSuperAdmin: u.IsSuperAdmin,
		TenantName:   tenantName,
		CreatedAt:    u.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// GetProfile returns the current user's full profile from the DB, including
// the tenant (team) name for display.
func (h *Handler) GetProfile(c *fiber.Ctx) error {
	u, err := h.repo.FindByID(middleware.UserID(c))
	if err != nil {
		return response.Fail(c, errs.NotFound("user not found"))
	}
	tenantName := ""
	if h.tenantRepo != nil {
		if t, err := h.tenantRepo.FindByID(u.TenantID); err == nil {
			tenantName = t.Name
		}
	}
	return response.OK(c, toView(u, tenantName))
}

// UpdateProfile lets the current user change their display name and
// preferred UI language. Language is optional; when provided it must be
// a supported locale ("en" or "zh").
func (h *Handler) UpdateProfile(c *fiber.Ctx) error {
	var body struct {
		Name     string `json:"name"`
		Language string `json:"language"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if body.Name == "" {
		return response.Fail(c, errs.BadRequest("name is required"))
	}
	switch body.Language {
	case "", "en", "zh":
	default:
		return response.Fail(c, errs.BadRequest("language must be en or zh"))
	}
	if err := h.repo.UpdateProfile(middleware.UserID(c), body.Name, body.Language); err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "update profile", err))
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}

// ChangePassword verifies the old password and sets a new one.
func (h *Handler) ChangePassword(c *fiber.Ctx) error {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if body.OldPassword == "" || body.NewPassword == "" {
		return response.Fail(c, errs.BadRequest("old_password and new_password are required"))
	}
	if len(body.NewPassword) < 6 {
		return response.Fail(c, errs.BadRequest("password must be at least 6 characters"))
	}

	u, err := h.repo.FindByID(middleware.UserID(c))
	if err != nil {
		return response.Fail(c, errs.NotFound("user not found"))
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(body.OldPassword)) != nil {
		return response.Fail(c, errs.Unauthorized("old password is incorrect"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "hash password", err))
	}
	if err := h.repo.UpdatePassword(u.ID, string(hash)); err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "update password", err))
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}

// ListUsers returns all users in the current tenant. Admin only.
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	tid := middleware.TenantID(c)
	users, err := h.repo.ListByTenant(tid)
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "list users", err))
	}
	tenantName := ""
	if h.tenantRepo != nil {
		if t, err := h.tenantRepo.FindByID(tid); err == nil {
			tenantName = t.Name
		}
	}
	// Batch per-user daily usage in one Redis round-trip.
	var usage map[string]int
	if h.messageUsed != nil {
		ids := make([]string, len(users))
		for i, u := range users {
			ids[i] = u.ID
		}
		usage = h.messageUsed(ids)
	}
	views := make([]*UserView, 0, len(users))
	for _, u := range users {
		v := toView(u, tenantName)
		if usage != nil {
			v.MessageUsed = usage[u.ID]
		}
		views = append(views, v)
	}
	return response.OK(c, fiber.Map{"items": views})
}

// UpdateRole changes a user's tenant-level role. Admin only. Prevents
// demoting the last admin or changing own role (use another admin).
func (h *Handler) UpdateRole(c *fiber.Ctx) error {
	var body struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if body.Role != RoleAdmin && body.Role != RoleMember {
		return response.Fail(c, errs.BadRequest("role must be admin or member"))
	}

	tenantID := middleware.TenantID(c)
	targetID := c.Params("userId")
	currentID := middleware.UserID(c)

	if targetID == currentID {
		return response.Fail(c, errs.BadRequest("cannot change own role"))
	}

	target, err := h.repo.FindByID(targetID)
	if err != nil || target.TenantID != tenantID {
		return response.Fail(c, errs.NotFound("user not found"))
	}

	// Prevent demoting the last admin.
	if target.Role == RoleAdmin && body.Role != RoleAdmin {
		count, err := h.repo.CountByTenant(tenantID)
		if err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "count admins", err))
		}
		if count <= 1 {
			return response.Fail(c, errs.BadRequest("cannot demote the last admin"))
		}
	}

	if err := h.repo.UpdateRole(tenantID, targetID, body.Role); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}

// UpdateStatus activates or deactivates a user. Admin only. Prevents
// disabling self or the last admin.
func (h *Handler) UpdateStatus(c *fiber.Ctx) error {
	var body struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if body.Status != StatusActive && body.Status != StatusDisabled {
		return response.Fail(c, errs.BadRequest("status must be active or disabled"))
	}

	tenantID := middleware.TenantID(c)
	targetID := c.Params("userId")
	currentID := middleware.UserID(c)

	if targetID == currentID {
		return response.Fail(c, errs.BadRequest("cannot change own status"))
	}

	target, err := h.repo.FindByID(targetID)
	if err != nil || target.TenantID != tenantID {
		return response.Fail(c, errs.NotFound("user not found"))
	}

	if target.Role == RoleAdmin && body.Status == StatusDisabled {
		count, err := h.repo.CountByTenant(tenantID)
		if err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "count admins", err))
		}
		if count <= 1 {
			return response.Fail(c, errs.BadRequest("cannot disable the last admin"))
		}
	}

	if err := h.repo.UpdateStatus(tenantID, targetID, body.Status); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}

// ResetPassword lets an admin set a new password for a tenant member.
// Prevents resetting own password (use ChangePassword for that).
func (h *Handler) ResetPassword(c *fiber.Ctx) error {
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if body.NewPassword == "" {
		return response.Fail(c, errs.BadRequest("new_password is required"))
	}
	if len(body.NewPassword) < 6 {
		return response.Fail(c, errs.BadRequest("password must be at least 6 characters"))
	}

	tenantID := middleware.TenantID(c)
	targetID := c.Params("userId")
	currentID := middleware.UserID(c)

	if targetID == currentID {
		return response.Fail(c, errs.BadRequest("cannot reset own password"))
	}

	target, err := h.repo.FindByID(targetID)
	if err != nil || target.TenantID != tenantID {
		return response.Fail(c, errs.NotFound("user not found"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "hash password", err))
	}
	if err := h.repo.UpdatePassword(targetID, string(hash)); err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "update password", err))
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}

// ListAllUsers returns all users across all tenants with their tenant names.
// Super admin only.
func (h *Handler) ListAllUsers(c *fiber.Ctx) error {
	users, err := h.repo.ListAll()
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "list users", err))
	}
	// Build tenant ID -> name map in one query.
	tenantNames := make(map[string]string)
	if h.tenantRepo != nil {
		if ts, err := h.tenantRepo.ListAll(); err == nil {
			for _, t := range ts {
				tenantNames[t.ID] = t.Name
			}
		}
	}
	views := make([]*UserView, 0, len(users))
	for _, u := range users {
		views = append(views, toView(u, tenantNames[u.TenantID]))
	}
	return response.OK(c, fiber.Map{"items": views})
}

// UpdateSuperAdmin grants or revokes the super-admin flag. Super admin only.
// Prevents revoking own flag or the last super admin.
func (h *Handler) UpdateSuperAdmin(c *fiber.Ctx) error {
	var body struct {
		IsSuperAdmin bool `json:"is_super_admin"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}

	targetID := c.Params("userId")
	currentID := middleware.UserID(c)

	if targetID == currentID {
		return response.Fail(c, errs.BadRequest("cannot change own super admin status"))
	}

	if _, err := h.repo.FindByID(targetID); err != nil {
		return response.Fail(c, errs.NotFound("user not found"))
	}

	// Prevent revoking the last super admin.
	if !body.IsSuperAdmin {
		count, err := h.repo.CountSuperAdmins()
		if err != nil {
			return response.Fail(c, errs.Wrap(errs.CodeInternal, "count super admins", err))
		}
		if count <= 1 {
			return response.Fail(c, errs.BadRequest("cannot revoke the last super admin"))
		}
	}

	if err := h.repo.UpdateSuperAdmin(targetID, body.IsSuperAdmin); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "updated"})
}
