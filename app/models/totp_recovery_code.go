package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type TOTPRecoveryCode struct {
	ID        airwaysql.IdType `db:"id" json:"id"`
	UserID    airwaysql.IdType `db:"user_id" json:"user_id"`
	CodeHash  string           `db:"code_hash" json:"-"`
	UsedAt    *time.Time       `db:"used_at" json:"used_at"`
	CreatedAt time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt time.Time        `db:"updated_at" json:"updated_at"`
}

func (TOTPRecoveryCode) TableName() string {
	return "totp_recovery_codes"
}

func init() {
	registerREPLModel("TOTPRecoveryCode", TOTPRecoveryCode{})
}
