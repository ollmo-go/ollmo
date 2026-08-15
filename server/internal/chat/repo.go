package chat

import (
	"errors"

	"gorm.io/gorm"

	"ollmo/ollmo/pkg/errs"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) CreateConv(c *Conversation) error { return r.db.Create(c).Error }

func (r *Repo) FindConv(tenantID, id string) (*Conversation, error) {
	var c Conversation
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("conversation not found")
		}
		return nil, err
	}
	return &c, nil
}

// FindConvOwned is like FindConv but also checks the caller owns the
// conversation. Returns NotFound when the id exists but belongs to another
// user, so the caller cannot infer the conversation exists.
func (r *Repo) FindConvOwned(tenantID, ownerID, id string) (*Conversation, error) {
	var c Conversation
	err := r.db.Where("tenant_id = ? AND owner_id = ? AND id = ?", tenantID, ownerID, id).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("conversation not found")
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repo) ListConvs(tenantID, ownerID string, page, size int) ([]*Conversation, int64, error) {
	var items []*Conversation
	var total int64
	q := r.db.Model(&Conversation{}).Where("tenant_id = ? AND owner_id = ?", tenantID, ownerID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("pinned DESC, updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}

// SearchConvs returns conversations whose title matches the query, or that
// contain a message matching the query. Results are deduped by conversation
// id and ordered pinned-first then by updated_at.
func (r *Repo) SearchConvs(tenantID, ownerID, query string, page, size int) ([]*Conversation, int64, error) {
	var items []*Conversation
	var total int64
	like := "%" + query + "%"
	q := r.db.Model(&Conversation{}).
		Where("tenant_id = ? AND owner_id = ?", tenantID, ownerID).
		Where("title LIKE ? OR id IN (?)",
			like,
			r.db.Model(&Message{}).Select("conversation_id").Where("tenant_id = ? AND content LIKE ?", tenantID, like))
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("pinned DESC, updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}

func (r *Repo) UpdateConvTitle(tenantID, id, title string) error {
	return r.db.Model(&Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("title", title).Error
}

func (r *Repo) UpdateConvPinned(tenantID, id string, pinned bool) error {
	return r.db.Model(&Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("pinned", pinned).Error
}

func (r *Repo) TouchConv(tenantID, id string) error {
	// bumped implicitly by autoUpdateTime on related writes; kept for explicit bumps
	return r.db.Model(&Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		UpdateColumn("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

func (r *Repo) DeleteConv(tenantID, ownerID, id string) error {
	// No FK cascade; delete the conversation's messages explicitly first.
	if err := r.db.Where("tenant_id = ? AND conversation_id = ?", tenantID, id).Delete(&Message{}).Error; err != nil {
		return err
	}
	res := r.db.Where("tenant_id = ? AND owner_id = ? AND id = ?", tenantID, ownerID, id).Delete(&Conversation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("conversation not found")
	}
	return nil
}

func (r *Repo) CreateMsg(m *Message) error { return r.db.Create(m).Error }

// SetMsgVote stores the user's feedback (""|"up"|"down") on one assistant
// message. Scoped to tenant + conversation; RowsAffected==0 means the message
// does not exist in that conversation (or is not an assistant message).
func (r *Repo) SetMsgVote(tenantID, convID, msgID, vote string) error {
	res := r.db.Model(&Message{}).
		Where("tenant_id = ? AND conversation_id = ? AND id = ? AND role = ?", tenantID, convID, msgID, RoleAssistant).
		Update("vote", vote)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("message not found")
	}
	return nil
}

func (r *Repo) ListMsgs(tenantID, convID string) ([]*Message, error) {
	var items []*Message
	err := r.db.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Order("created_at ASC").
		Find(&items).Error
	return items, err
}

// RecentMsgs returns the last N messages in chronological order. Used to
// build the LLM context window without loading the entire history.
func (r *Repo) RecentMsgs(tenantID, convID string, limit int) ([]*Message, error) {
	var items []*Message
	sub := r.db.Model(&Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Order("created_at DESC").
		Limit(limit)
	err := r.db.Raw("SELECT * FROM (?) AS sub ORDER BY created_at ASC", sub).Scan(&items).Error
	return items, err
}

// CountMsgs returns the number of messages in a conversation. Used by the
// auto-summarization trigger to decide when a conversation has grown enough
// to deserve a fresh summary.
func (r *Repo) CountMsgs(tenantID, convID string) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Count(&n).Error
	return n, err
}
