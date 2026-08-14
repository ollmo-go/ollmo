package invitation

import (
	"errors"

	"gorm.io/gorm"
	"ollmo/ollmo/pkg/errs"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(inv *Invitation) error {
	return r.db.Create(inv).Error
}

func (r *Repo) FindByToken(token string) (*Invitation, error) {
	var inv Invitation
	err := r.db.Where("token = ?", token).First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *Repo) FindByID(tenantID, id string) (*Invitation, error) {
	var inv Invitation
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *Repo) ListByTenant(tenantID string) ([]*Invitation, error) {
	var invs []*Invitation
	err := r.db.Where("tenant_id = ? AND status = ?", tenantID, StatusPending).
		Order("created_at DESC").Find(&invs).Error
	return invs, err
}

func (r *Repo) UpdateStatus(token, status string) error {
	return r.db.Model(&Invitation{}).
		Where("token = ?", token).
		Update("status", status).Error
}

func (r *Repo) MarkAccepted(token string, acceptedAt interface{}) error {
	return r.db.Model(&Invitation{}).
		Where("token = ?", token).
		Updates(map[string]interface{}{
			"status":      StatusAccepted,
			"accepted_at": acceptedAt,
		}).Error
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ? AND status = ?", tenantID, id, StatusPending).
		Delete(&Invitation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("invitation not found")
	}
	return nil
}

func (r *Repo) FindPendingByEmail(tenantID, email string) (*Invitation, error) {
	var inv Invitation
	err := r.db.Where("tenant_id = ? AND email = ? AND status = ?", tenantID, email, StatusPending).
		First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &inv, nil
}
