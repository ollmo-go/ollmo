package audit

import "time"

// AuditLog records a write operation performed by a user. Read operations
// are not audited to keep the volume manageable.
type AuditLog struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID  string    `gorm:"size:36;index:idx_audit_tenant_created,priority:1;not null" json:"tenant_id"`
	UserID    string    `gorm:"size:36;not null" json:"user_id"`
	UserName  string    `gorm:"size:128" json:"user_name"`
	Action    string    `gorm:"size:64;index;not null" json:"action"` // e.g. "kb.create", "doc.delete"
	Resource  string    `gorm:"size:36" json:"resource"`
	Detail    string    `gorm:"type:text" json:"detail,omitempty"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `gorm:"index:idx_audit_tenant_created,priority:2" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }
