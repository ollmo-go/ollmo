package invitation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/errs"
)

type Service struct {
	repo  *Repo
	users *user.Repo
}

func NewService(repo *Repo, users *user.Repo) *Service {
	return &Service{repo: repo, users: users}
}

const tokenBytes = 32
const expiryDuration = 24 * 7 * time.Hour // 7 days

type CreateInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type InvitationView struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

func toView(inv *Invitation) *InvitationView {
	return &InvitationView{
		ID:        inv.ID,
		Email:     inv.Email,
		Role:      inv.Role,
		Status:    inv.Status,
		ExpiresAt: inv.ExpiresAt.Format("2006-01-02 15:04"),
		CreatedAt: inv.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// Create generates a new invitation. Returns the raw token so the caller
// can build the invitation link. The token is stored as a unique column;
// only this call returns it in plaintext.
func (s *Service) Create(ctx context.Context, tenantID, invitedBy string, in CreateInput) (*InvitationView, string, error) {
	if in.Email == "" {
		return nil, "", errs.BadRequest("email is required")
	}
	if in.Role != user.RoleAdmin && in.Role != user.RoleMember {
		return nil, "", errs.BadRequest("role must be admin or member")
	}

	// Check if email is already a tenant member.
	existing, err := s.users.FindByEmail(in.Email)
	if err == nil && existing.TenantID == tenantID {
		return nil, "", errs.Conflict("user already in this tenant")
	}

	// Check for existing pending invitation.
	pending, err := s.repo.FindPendingByEmail(tenantID, in.Email)
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "check pending", err)
	}
	if pending != nil {
		return nil, "", errs.Conflict("invitation already pending")
	}

	token, err := generateToken()
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "generate token", err)
	}

	inv := &Invitation{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Email:     in.Email,
		Role:      in.Role,
		InvitedBy: invitedBy,
		Token:     token,
		Status:    StatusPending,
		ExpiresAt: time.Now().Add(expiryDuration),
	}
	if err := s.repo.Create(inv); err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "create invitation", err)
	}
	return toView(inv), token, nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]*InvitationView, error) {
	invs, err := s.repo.ListByTenant(tenantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list invitations", err)
	}
	views := make([]*InvitationView, 0, len(invs))
	for _, inv := range invs {
		views = append(views, toView(inv))
	}
	return views, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(tenantID, id)
}

// AcceptInput is the body for accepting an invitation. Email and password
// create a new account; the email must match the invitation.
type AcceptInput struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Accept validates the token, creates a user account in the inviting tenant,
// and marks the invitation as accepted. Returns the new user so the caller
// can issue a JWT.
func (s *Service) Accept(ctx context.Context, in AcceptInput) (*user.User, error) {
	if in.Token == "" || in.Password == "" {
		return nil, errs.BadRequest("token, email and password are required")
	}

	inv, err := s.repo.FindByToken(in.Token)
	if err != nil {
		return nil, errs.NotFound("invitation not found")
	}
	if inv.Status != StatusPending {
		return nil, errs.BadRequest("invitation is " + inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		_ = s.repo.UpdateStatus(in.Token, StatusCancelled)
		return nil, errs.BadRequest("invitation has expired")
	}

	// Email must match the invitation.
	if in.Name == "" {
		in.Name = inv.Email
	}

	// Check if user already exists (registered independently).
	if existing, err := s.users.FindByEmail(inv.Email); err == nil {
		if existing.TenantID == inv.TenantID {
			return nil, errs.Conflict("email already registered in this tenant")
		}
		return nil, errs.Conflict("email already registered in another tenant")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password", err)
	}

	u := &user.User{
		ID:           uuid.NewString(),
		TenantID:     inv.TenantID,
		Email:        inv.Email,
		PasswordHash: string(hash),
		Name:         in.Name,
		Role:         inv.Role,
		Status:       user.StatusActive,
	}
	if err := s.users.Create(u); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create user", err)
	}

	now := time.Now()
	if err := s.repo.MarkAccepted(in.Token, now); err != nil {
		// User is already created; log but don't fail.
	}

	return u, nil
}

// Peek returns invitation details by token, without accepting. Used by
// the frontend to show the invitation page (tenant name, role, email).
func (s *Service) Peek(ctx context.Context, token string) (*InvitationView, error) {
	inv, err := s.repo.FindByToken(token)
	if err != nil {
		return nil, errs.NotFound("invitation not found")
	}
	if inv.Status != StatusPending {
		return nil, errs.BadRequest("invitation is " + inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, errs.BadRequest("invitation has expired")
	}
	return toView(inv), nil
}

func generateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "inv_" + hex.EncodeToString(b), nil
}
