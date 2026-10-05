package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type APIToken struct {
	ID         airwaysql.IdType `db:"id" json:"id"`
	UserID     airwaysql.IdType `db:"user_id" json:"user_id"`
	Name       string           `db:"name" json:"name"`
	TokenHash  string           `db:"token_hash" json:"-"`
	Scopes     string           `db:"scopes" json:"scopes"`
	ExpiresAt  *time.Time       `db:"expires_at" json:"expires_at"`
	LastUsedAt *time.Time       `db:"last_used_at" json:"last_used_at"`
	CreatedAt  time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time        `db:"updated_at" json:"updated_at"`
}

func (APIToken) TableName() string {
	return "api_tokens"
}

func init() {
	registerREPLModel("APIToken", APIToken{})
}
