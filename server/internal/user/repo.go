package user

import (
	"ollmo/ollmo/pkg/errs"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(u *User) error { return r.db.Create(u).Error }

func (r *Repo) FindByEmail(email string) (*User, error) {
	var u User
	err := r.db.Where("email = ?", email).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) FindByID(id string) (*User, error) {
	var u User
	err := r.db.Where("id = ?", id).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) FindByIDs(ids []string) (map[string]*User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []*User
	if err := r.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	m := make(map[string]*User, len(users))
	for _, u := range users {
		m[u.ID] = u
	}
	return m, nil
}

func (r *Repo) ListByTenant(tenantID string) ([]*User, error) {
	var users []*User
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at ASC").Find(&users).Error
	return users, err
}

func (r *Repo) UpdateRole(tenantID, userID, role string) error {
	res := r.db.Model(&User{}).
		Where("tenant_id = ? AND id = ?", tenantID, userID).
		Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("user not found")
	}
	return nil
}

func (r *Repo) UpdateStatus(tenantID, userID, status string) error {
	res := r.db.Model(&User{}).
		Where("tenant_id = ? AND id = ?", tenantID, userID).
		Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("user not found")
	}
	return nil
}

func (r *Repo) CountByTenant(tenantID string) (int64, error) {
	var count int64
	err := r.db.Model(&User{}).Where("tenant_id = ? AND status = ?", tenantID, StatusActive).Count(&count).Error
	return count, err
}

// CountByTenants returns active member counts grouped by tenant ID in a
// single query. Used by the super-admin tenant list to avoid N+1 queries.
func (r *Repo) CountByTenants() (map[string]int64, error) {
	type row struct {
		TenantID string `gorm:"column:tenant_id"`
		Count    int64  `gorm:"column:count"`
	}
	var rows []row
	if err := r.db.Model(&User{}).Select("tenant_id, COUNT(*) AS count").Where("status = ?", StatusActive).Group("tenant_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, r := range rows {
		m[r.TenantID] = r.Count
	}
	return m, nil
}

// UpdateProfile updates the user's display name and language. Email is
// identity and stays read-only.
func (r *Repo) UpdateProfile(userID, name, language string) error {
	res := r.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"name":     name,
		"language": language,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("user not found")
	}
	return nil
}

// UpdatePassword sets a new bcrypt password hash for the user.
func (r *Repo) UpdatePassword(userID, passwordHash string) error {
	res := r.db.Model(&User{}).Where("id = ?", userID).Update("password_hash", passwordHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("user not found")
	}
	return nil
}

// ListAll returns all users across all tenants (super-admin user management).
func (r *Repo) ListAll() ([]*User, error) {
	var users []*User
	err := r.db.Order("created_at ASC").Find(&users).Error
	return users, err
}

// UpdateSuperAdmin grants or revokes the super-admin flag of a user.
func (r *Repo) UpdateSuperAdmin(userID string, isSuperAdmin bool) error {
	res := r.db.Model(&User{}).Where("id = ?", userID).Update("is_super_admin", isSuperAdmin)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("user not found")
	}
	return nil
}

// CountSuperAdmins returns the number of super admins (used to prevent
// revoking the last one).
func (r *Repo) CountSuperAdmins() (int64, error) {
	var count int64
	err := r.db.Model(&User{}).Where("is_super_admin = ?", true).Count(&count).Error
	return count, err
}
