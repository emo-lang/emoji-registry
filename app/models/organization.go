package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type Organization struct {
	ID          airwaysql.IdType `db:"id" json:"id"`
	Name        string           `db:"name" json:"name"`
	DisplayName string           `db:"display_name" json:"display_name"`
	UserID      airwaysql.IdType `db:"user_id" json:"user_id"`
	CreatedAt   time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time        `db:"updated_at" json:"updated_at"`
}

func (Organization) TableName() string {
	return "organizations"
}

func init() {
	registerREPLModel("Organization", Organization{})
}
