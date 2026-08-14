package auth

import (
	"context"
	"log"
	"time"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/internal/tenant"
	"ollmo/ollmo/internal/user"
	"ollmo/ollmo/pkg/errs"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	users     *user.Repo
	tenants   *tenant.Repo
	jwtSecret string
	jwtExpire int
	quotas    QuotaResolver
	revoker   middleware.TokenRevoker
	attempts  LoginAttemptStore
	regCheck  RegistrationChecker
}

// QuotaResolver returns resource limits for a plan name. Wired to
// config.QuotaConfig by the server composition layer.
type QuotaResolver interface {
	QuotasFor(plan string) (docQuota, vectorQuota, messageQuota, userMessageQuota int)
}

// RegistrationChecker reports whether self-service registration is enabled.
// Wired to site.Service by the server composition layer.
type RegistrationChecker interface {
	RegistrationAllowed() bool
}

// LoginAttemptStore tracks per-account login failures for brute-force
// protection. A Redis implementation is provided; the interface allows
// alternatives (e.g. in-memory for tests).
type LoginAttemptStore interface {
	FailureCount(ctx context.Context, email string) (int, error)
	RecordFailure(ctx context.Context, email string) error
	Reset(ctx context.Context, email string) error
}

const maxLoginAttempts = 5

// RedisLoginAttempts implements LoginAttemptStore using Redis. Failed attempts
// expire after 15 minutes so the lockout is temporary.
type RedisLoginAttempts struct{ rdb *redis.Client }

func NewRedisLoginAttempts(rdb *redis.Client) *RedisLoginAttempts {
	return &RedisLoginAttempts{rdb: rdb}
}

func (s *RedisLoginAttempts) FailureCount(ctx context.Context, email string) (int, error) {
	n, err := s.rdb.Get(ctx, "login:fail:"+email).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

func (s *RedisLoginAttempts) RecordFailure(ctx context.Context, email string) error {
	key := "login:fail:" + email
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 15*time.Minute)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisLoginAttempts) Reset(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, "login:fail:"+email).Err()
}

func NewService(users *user.Repo, tenants *tenant.Repo, jwtSecret string, jwtExpireHours int) *Service {
	return &Service{users: users, tenants: tenants, jwtSecret: jwtSecret, jwtExpire: jwtExpireHours}
}

// WithQuotas injects plan-based quota resolution. When unset, Register
// falls back to the Tenant model's GORM defaults (100 docs / 10000 vectors).
func (s *Service) WithQuotas(q QuotaResolver) *Service {
	s.quotas = q
	return s
}

// WithRevoker wires JWT token revocation (Redis-backed). When set, Logout
// adds the token's jti to the revocation set so it can't be reused.
func (s *Service) WithRevoker(r middleware.TokenRevoker) *Service {
	s.revoker = r
	return s
}

// WithLoginAttempts wires per-account login failure tracking (Redis-backed).
// When set, Login enforces a temporary lockout after too many failures.
func (s *Service) WithLoginAttempts(a LoginAttemptStore) *Service {
	s.attempts = a
	return s
}

// WithRegistrationChecker wires the site registration toggle. When set and
// registration is disabled, Register rejects new signups.
func (s *Service) WithRegistrationChecker(r RegistrationChecker) *Service {
	s.regCheck = r
	return s
}

type RegisterInput struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	Name       string `json:"name"`
	TenantName string `json:"tenant_name"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	UserID    string `json:"user_id"`
	TenantID  string `json:"tenant_id"`
}

// Register creates a new tenant plus its admin user (one tenant per
// registration). This is the only endpoint that creates a tenant.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*TokenResponse, error) {
	if s.regCheck != nil && !s.regCheck.RegistrationAllowed() {
		return nil, errs.Forbidden("registration is disabled")
	}
	if in.Email == "" || in.Password == "" {
		return nil, errs.BadRequest("email and password required")
	}
	if _, err := s.users.FindByEmail(in.Email); err == nil {
		return nil, errs.Conflict("email already registered")
	}

	plan := "free"
	docQuota, vectorQuota, messageQuota, userMessageQuota := 100, 10000, 100, 20
	if s.quotas != nil {
		docQuota, vectorQuota, messageQuota, userMessageQuota = s.quotas.QuotasFor(plan)
	}
	t := &tenant.Tenant{
		ID:               uuid.NewString(),
		Name:             ifEmpty(in.TenantName, in.Email+"'s workspace"),
		Plan:             plan,
		DocQuota:         docQuota,
		VectorQuota:      vectorQuota,
		MessageQuota:     messageQuota,
		UserMessageQuota: userMessageQuota,
	}
	if err := s.tenants.Create(t); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create tenant", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "hash password", err)
	}
	u := &user.User{
		ID:           uuid.NewString(),
		TenantID:     t.ID,
		Email:        in.Email,
		PasswordHash: string(hash),
		Name:         ifEmpty(in.Name, in.Email),
		Role:         "admin",
		Status:       "active",
	}
	if err := s.users.Create(u); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create user", err)
	}

	// Record the user as owner of the new tenant. The membership table is
	// the future extension point for multi-tenant joins; today it simply
	// mirrors the 1:1 user-tenant relationship.
	member := &tenant.TenantMember{
		ID:       uuid.NewString(),
		TenantID: t.ID,
		UserID:   u.ID,
		Role:     tenant.RoleOwner,
		Current:  true,
	}
	if err := s.tenants.CreateMember(member); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create tenant member", err)
	}
	return s.issueToken(u)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenResponse, error) {
	// Brute-force protection: reject if too many recent failures.
	if s.attempts != nil {
		if count, _ := s.attempts.FailureCount(ctx, in.Email); count >= maxLoginAttempts {
			return nil, errs.Forbidden("too many failed attempts, try again later")
		}
	}

	u, err := s.users.FindByEmail(in.Email)
	if err != nil {
		s.recordFailure(ctx, in.Email)
		return nil, errs.Unauthorized("invalid email or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)); err != nil {
		s.recordFailure(ctx, in.Email)
		return nil, errs.Unauthorized("invalid email or password")
	}
	if u.Status != "active" {
		return nil, errs.Forbidden("user is " + u.Status)
	}
	// Clear failure counter on success.
	if s.attempts != nil {
		_ = s.attempts.Reset(ctx, in.Email)
	}
	return s.issueToken(u)
}

func (s *Service) recordFailure(ctx context.Context, email string) {
	if s.attempts == nil {
		return
	}
	if err := s.attempts.RecordFailure(ctx, email); err != nil {
		log.Printf("[auth] record login failure failed email=%s: %v", email, err)
	}
}

// Logout revokes the JWT by adding its jti to the revocation set with a TTL
// equal to the token's remaining lifetime. After this, the Auth middleware
// rejects the token even though it hasn't expired yet.
func (s *Service) Logout(ctx context.Context, tokenStr string) error {
	if s.revoker == nil {
		return nil
	}
	claims := &middleware.Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return nil // expired/invalid: nothing to revoke, cookie still clears
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl > 0 {
		return s.revoker.Revoke(ctx, claims.ID, ttl)
	}
	return nil
}

func (s *Service) issueToken(u *user.User) (*TokenResponse, error) {
	token, err := middleware.IssueToken(s.jwtSecret, u.ID, u.TenantID, u.Role, u.IsSuperAdmin, s.jwtExpire)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "sign jwt", err)
	}
	return &TokenResponse{
		Token:     token,
		ExpiresIn: s.jwtExpire * 3600,
		UserID:    u.ID,
		TenantID:  u.TenantID,
	}, nil
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
