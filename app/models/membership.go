package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

// Membership roles.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Membership struct {
	ID             airwaysql.IdType `db:"id" json:"id"`
	OrganizationID airwaysql.IdType `db:"organization_id" json:"organization_id"`
	UserID         airwaysql.IdType `db:"user_id" json:"user_id"`
	Role           string           `db:"role" json:"role"`
	CreatedAt      time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time        `db:"updated_at" json:"updated_at"`
}

func (Membership) TableName() string {
	return "memberships"
}

func init() {
	registerREPLModel("Membership", Membership{})
}
