package user

import "time"

// User is a local account authenticated via email + bcrypt password.
type User struct {
	ID           string `gorm:"primaryKey;size:36" json:"id"`
	TenantID     string `gorm:"size:36;index;not null" json:"tenant_id"`
	Email        string `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	Name         string `gorm:"size:128;not null" json:"name"`
	// Language is the user's preferred UI locale ("en" or "zh"). Empty
	// means no preference; the frontend falls back to its own default.
	Language     string    `gorm:"size:8;not null;default:''" json:"language"`
	Role         string    `gorm:"size:32;not null;default:member" json:"role"`
	Status       string    `gorm:"size:32;not null;default:active" json:"status"`
	IsSuperAdmin bool      `gorm:"not null;default:false" json:"is_super_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

const (
	RoleAdmin      = "admin"
	RoleMember     = "member"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)
