package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/errs"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// Create generates a new API key. The returned fullKey is the raw key string
// and is shown to the caller only once; only the hash and prefix are persisted.
func (s *Service) Create(tenantID, userID, name string, expiresAt *time.Time) (*APIKey, string, error) {
	if name == "" {
		return nil, "", errs.BadRequest("name is required")
	}
	fullKey, prefix, hash, err := generateKey()
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "generate api key", err)
	}
	k := &APIKey{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    userID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Status:    StatusActive,
		ExpiresAt: expiresAt,
	}
	if err := s.repo.Create(k); err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "create api key", err)
	}
	return k, fullKey, nil
}

// List returns API keys. Admins see all tenant keys; regular users see only
// their own.
func (s *Service) List(tenantID, userID string, isAdmin bool) ([]*APIKey, error) {
	uid := userID
	if isAdmin {
		uid = ""
	}
	return s.repo.List(tenantID, uid)
}

func (s *Service) Revoke(tenantID, userID, id string, isAdmin bool) error {
	uid := userID
	if isAdmin {
		uid = ""
	}
	return s.repo.Revoke(tenantID, uid, id)
}

func (s *Service) Delete(tenantID, userID, id string, isAdmin bool) error {
	uid := userID
	if isAdmin {
		uid = ""
	}
	return s.repo.Delete(tenantID, uid, id)
}

// Verify validates a raw API key, returning the matching record on success.
// It checks the prefix, hash, status, and expiry, and updates last_used_at.
func (s *Service) Verify(rawKey string) (*APIKey, error) {
	if len(rawKey) != KeyTotalLen || !strings.HasPrefix(rawKey, KeyPrefix) {
		return nil, errs.Unauthorized("invalid api key")
	}
	prefix := rawKey[:12]
	hash := hashKey(rawKey)

	k, err := s.repo.FindByPrefix("", prefix)
	if err != nil {
		return nil, errs.Unauthorized("invalid api key")
	}
	if subtle.ConstantTimeCompare([]byte(k.KeyHash), []byte(hash)) != 1 {
		return nil, errs.Unauthorized("invalid api key")
	}
	if k.Status != StatusActive {
		return nil, errs.Unauthorized("api key revoked")
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return nil, errs.Unauthorized("api key expired")
	}
	// Best-effort: a failure to update last_used_at must not fail the request.
	if err := s.repo.UpdateLastUsed(k.ID); err != nil {
		log.Printf("[apikey] update last_used failed id=%s: %v", k.ID, err)
	}
	return k, nil
}

// generateKey produces a random API key. The key is KeyPrefix + 36 hex chars
// (18 random bytes), yielding KeyTotalLen (40) characters total.
func generateKey() (fullKey, prefix, hash string, err error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", err
	}
	fullKey = KeyPrefix + hex.EncodeToString(b)
	prefix = fullKey[:12]
	hash = hashKey(fullKey)
	return fullKey, prefix, hash, nil
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
