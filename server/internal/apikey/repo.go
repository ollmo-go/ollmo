package apikey

import (
	"errors"
	"time"

	"ollmo/ollmo/pkg/errs"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(k *APIKey) error { return r.db.Create(k).Error }

// FindByPrefix looks up a key by its prefix. When tenantID is empty the
// search spans all tenants (used by the auth middleware where the tenant
// is not yet known).
func (r *Repo) FindByPrefix(tenantID, prefix string) (*APIKey, error) {
	var k APIKey
	q := r.db.Where("key_prefix = ?", prefix)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("api key not found")
		}
		return nil, err
	}
	return &k, nil
}

// List returns API keys for a tenant. When userID is non-empty, only the
// caller's own keys are returned (regular users); empty userID returns all
// tenant keys (admin).
func (r *Repo) List(tenantID, userID string) ([]*APIKey, error) {
	var items []*APIKey
	q := r.db.Where("tenant_id = ?", tenantID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("created_at DESC").Find(&items).Error
	return items, err
}

func (r *Repo) UpdateLastUsed(id string) error {
	return r.db.Model(&APIKey{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now()).Error
}

// UserRole resolves the key owner's role so API-key requests carry the same
// identity as JWT auth for downstream gating (kbAccess, AdminOnly).
func (r *Repo) UserRole(tenantID, userID string) (role string, superAdmin bool) {
	var row struct {
		Role         string
		IsSuperAdmin bool
	}
	if err := r.db.Table("users").
		Select("role, is_super_admin").
		Where("tenant_id = ? AND id = ?", tenantID, userID).
		Scan(&row).Error; err != nil {
		return "", false
	}
	return row.Role, row.IsSuperAdmin
}

// Delete removes an API key. When userID is non-empty, the key must belong
// to that user; empty userID allows deleting any tenant key (admin).
func (r *Repo) Delete(tenantID, userID, id string) error {
	q := r.db.Where("tenant_id = ? AND id = ?", tenantID, id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Delete(&APIKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("api key not found")
	}
	return nil
}

// Revoke marks a key as revoked without deleting the row so usage history
// and audit trails are preserved. When userID is non-empty, the key must
// belong to that user; empty userID allows revoking any tenant key (admin).
func (r *Repo) Revoke(tenantID, userID, id string) error {
	q := r.db.Model(&APIKey{}).Where("tenant_id = ? AND id = ?", tenantID, id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Update("status", StatusRevoked)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("api key not found")
	}
	return nil
}
