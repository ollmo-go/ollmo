package apikey

import "time"

// APIKey stores a hashed API key for external programmatic access.
// The full key is shown only once at creation; key_prefix (first 12 chars)
// is stored for display and lookup.
type APIKey struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	TenantID   string     `gorm:"size:36;index;not null" json:"tenant_id"`
	UserID     string     `gorm:"size:36;not null" json:"user_id"`
	Name       string     `gorm:"size:128;not null" json:"name"`
	KeyPrefix  string     `gorm:"size:16;index;not null" json:"key_prefix"`
	KeyHash    string     `gorm:"size:128;not null" json:"-"`
	Status     string     `gorm:"size:16;not null;default:active" json:"status"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (APIKey) TableName() string { return "api_keys" }

const (
	StatusActive  = "active"
	StatusRevoked = "revoked"
	KeyPrefix     = "olk_" // ollmo key prefix
	KeyTotalLen   = 40     // olk_ + 36 hex chars
)
