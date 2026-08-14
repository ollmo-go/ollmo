package tenant

import "gorm.io/gorm"

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(t *Tenant) error { return r.db.Create(t).Error }

func (r *Repo) FindByID(id string) (*Tenant, error) {
	var t Tenant
	err := r.db.Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateName renames a tenant. Only the name field is updated.
func (r *Repo) UpdateName(tenantID, name string) error {
	res := r.db.Model(&Tenant{}).Where("id = ?", tenantID).Update("name", name)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountDocs returns the total document count for a tenant across all KBs.
func (r *Repo) CountDocs(tenantID string) (int64, error) {
	var count int64
	err := r.db.Table("documents").Where("tenant_id = ?", tenantID).Count(&count).Error
	return count, err
}

// CountDocsByTenant returns document counts grouped by tenant ID in a single
// query. Used by the super-admin tenant list to avoid N+1 queries.
func (r *Repo) CountDocsByTenant() (map[string]int64, error) {
	type row struct {
		TenantID string `gorm:"column:tenant_id"`
		Count    int64  `gorm:"column:count"`
	}
	var rows []row
	if err := r.db.Table("documents").Select("tenant_id, COUNT(*) AS count").Group("tenant_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, r := range rows {
		m[r.TenantID] = r.Count
	}
	return m, nil
}

// CountChunks returns the total chunk (vector) count for a tenant across all KBs.
func (r *Repo) CountChunks(tenantID string) (int64, error) {
	var count int64
	err := r.db.Table("chunks").Where("tenant_id = ?", tenantID).Count(&count).Error
	return count, err
}

// ListAll returns every tenant ordered by creation time. Super-admin only.
func (r *Repo) ListAll() ([]Tenant, error) {
	var ts []Tenant
	err := r.db.Order("created_at ASC").Find(&ts).Error
	return ts, err
}

// UpdateQuotas updates the quota fields of a tenant. Super-admin only.
func (r *Repo) UpdateQuotas(tenantID string, docQuota, vectorQuota, messageQuota, userMessageQuota int) error {
	res := r.db.Model(&Tenant{}).Where("id = ?", tenantID).Updates(map[string]any{
		"doc_quota":          docQuota,
		"vector_quota":       vectorQuota,
		"message_quota":      messageQuota,
		"user_message_quota": userMessageQuota,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdatePlan changes the tenant plan and applies the matching quotas.
// Super-admin only.
func (r *Repo) UpdatePlan(tenantID, plan string, docQuota, vectorQuota, messageQuota, userMessageQuota int) error {
	res := r.db.Model(&Tenant{}).Where("id = ?", tenantID).Updates(map[string]any{
		"plan":               plan,
		"doc_quota":          docQuota,
		"vector_quota":       vectorQuota,
		"message_quota":      messageQuota,
		"user_message_quota": userMessageQuota,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CreateMember inserts a tenant membership row. Called at registration to
// record the user as owner of their default tenant.
func (r *Repo) CreateMember(m *TenantMember) error { return r.db.Create(m).Error }
