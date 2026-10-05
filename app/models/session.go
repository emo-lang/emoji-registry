package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type Session struct {
	ID        airwaysql.IdType `db:"id" json:"id"`
	Token     string           `db:"token" json:"token"`
	UserID    airwaysql.IdType `db:"user_id" json:"user_id"`
	ExpiresAt time.Time        `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt time.Time        `db:"updated_at" json:"updated_at"`
}

func (Session) TableName() string {
	return "sessions"
}

func init() {
	registerREPLModel("Session", Session{})
}
