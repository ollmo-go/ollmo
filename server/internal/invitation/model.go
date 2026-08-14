package invitation

import "time"

const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusCancelled = "cancelled"
)

type Invitation struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	TenantID   string     `gorm:"size:36;not null;index" json:"tenant_id"`
	Email      string     `gorm:"size:255;not null" json:"email"`
	Role       string     `gorm:"size:16;not null;default:member" json:"role"`
	InvitedBy  string     `gorm:"size:36;not null" json:"invited_by"`
	Token      string     `gorm:"size:128;not null;uniqueIndex" json:"-"`
	Status     string     `gorm:"size:16;not null;default:pending;index" json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (Invitation) TableName() string { return "invitations" }
