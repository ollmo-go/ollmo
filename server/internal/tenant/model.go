package tenant

import "time"

// Tenant is the top-level isolation boundary. Every business row carries
// tenant_id for logical multi-tenant isolation (L1).
type Tenant struct {
	ID               string    `gorm:"primaryKey;size:36" json:"id"`
	Name             string    `gorm:"size:128;not null" json:"name"`
	Plan             string    `gorm:"size:32;not null;default:free" json:"plan"`
	DocQuota         int       `gorm:"not null;default:100" json:"doc_quota"`
	VectorQuota      int       `gorm:"not null;default:10000" json:"vector_quota"`
	MessageQuota     int       `gorm:"not null;default:100" json:"message_quota"`
	UserMessageQuota int       `gorm:"not null;default:20" json:"user_message_quota"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Tenant) TableName() string { return "tenants" }

// TenantMember links a user to a tenant with a role. Today the relationship
// is 1:1 (one user per tenant, created at registration); the join table is
// in place so multi-tenant membership can be added later without schema
// changes.
type TenantMember struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID  string    `gorm:"size:36;index;not null" json:"tenant_id"`
	UserID    string    `gorm:"size:36;index;not null" json:"user_id"`
	Role      string    `gorm:"size:32;not null;default:member" json:"role"`
	Current   bool      `gorm:"default:false" json:"current"`
	CreatedAt time.Time `json:"created_at"`
}

func (TenantMember) TableName() string { return "tenant_members" }

const RoleOwner = "owner"
