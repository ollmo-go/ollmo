package bill

import (
	"errors"

	"gorm.io/gorm"

	"ollmo/ollmo/pkg/errs"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(rec *Record) error { return r.db.Create(rec).Error }

func (r *Repo) FindByID(id string) (*Record, error) {
	var rec Record
	err := r.db.Where("id = ?", id).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("bill not found")
		}
		return nil, err
	}
	return &rec, nil
}

func (r *Repo) List(tenantID string, limit int) ([]*Record, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var items []*Record
	err := r.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// UserTotals sums token/cost per user. Left-joined to users so every row is
// attributed even when the user row was deleted. UserID may be "" (unscoped
// system calls); such rows are attributed to "(system)".
func (r *Repo) UserTotals(tenantID string) ([]UserTotal, error) {
	var items []UserTotal
	err := r.db.Raw(`
		SELECT b.user_id, COALESCE(u.name, '') AS user_name,
			SUM(b.prompt_tokens) AS prompt_tokens,
			SUM(b.completion_tokens) AS completion_tokens,
			SUM(b.total_tokens) AS total_tokens,
			SUM(b.amount) AS amount,
			COUNT(*) AS call_count
		FROM bills b
		LEFT JOIN users u ON u.id = b.user_id
		WHERE b.tenant_id = ?
		GROUP BY b.user_id, u.name
		ORDER BY total_tokens DESC`, tenantID).Scan(&items).Error
	return items, err
}

// ModelTotals sums token/cost per model, ranked by total tokens.
func (r *Repo) ModelTotals(tenantID string) ([]ModelTotal, error) {
	var items []ModelTotal
	err := r.db.Raw(`
		SELECT model_id, model_name,
			SUM(prompt_tokens) AS prompt_tokens,
			SUM(completion_tokens) AS completion_tokens,
			SUM(total_tokens) AS total_tokens,
			SUM(amount) AS amount,
			COUNT(*) AS call_count
		FROM bills
		WHERE tenant_id = ?
		GROUP BY model_id, model_name
		ORDER BY total_tokens DESC`, tenantID).Scan(&items).Error
	return items, err
}

// Overview returns the tenant's total tokens/cost/calls across all bills.
func (r *Repo) Overview(tenantID string) (*Overview, error) {
	var o Overview
	err := r.db.Raw(`
		SELECT COALESCE(SUM(prompt_tokens),0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens),0) AS completion_tokens,
			COALESCE(SUM(total_tokens),0) AS total_tokens,
			COALESCE(SUM(amount),0) AS amount,
			COUNT(*) AS call_count
		FROM bills
		WHERE tenant_id = ?`, tenantID).Scan(&o).Error
	return &o, err
}

// OverviewByUser returns one user's own totals, scoped to that user so
// members can see their personal consumption without other users' rows.
func (r *Repo) OverviewByUser(tenantID, userID string) (*Overview, error) {
	var o Overview
	err := r.db.Raw(`
		SELECT COALESCE(SUM(prompt_tokens),0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens),0) AS completion_tokens,
			COALESCE(SUM(total_tokens),0) AS total_tokens,
			COALESCE(SUM(amount),0) AS amount,
			COUNT(*) AS call_count
		FROM bills
		WHERE tenant_id = ? AND user_id = ?`, tenantID, userID).Scan(&o).Error
	return &o, err
}

// ListByUser returns the user's most recent bill rows (newest first).
func (r *Repo) ListByUser(tenantID, userID string, limit int) ([]*Record, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var items []*Record
	err := r.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}
