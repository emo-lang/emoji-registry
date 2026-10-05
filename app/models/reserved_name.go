package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type ReservedName struct {
	ID        airwaysql.IdType `db:"id" json:"id"`
	Name      string           `db:"name" json:"name"`
	Reason    string           `db:"reason" json:"reason"`
	CreatedAt time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt time.Time        `db:"updated_at" json:"updated_at"`
}

func (ReservedName) TableName() string {
	return "reserved_names"
}

func init() {
	registerREPLModel("ReservedName", ReservedName{})
}
