package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

// Download is the per-version, per-day download aggregate. Date is a
// "2006-01-02" day string so all dialects compare it identically.
type Download struct {
	ID        airwaysql.IdType `db:"id" json:"id"`
	VersionID airwaysql.IdType `db:"version_id" json:"version_id"`
	Date      string           `db:"date" json:"date"`
	Count     int64            `db:"count" json:"count"`
	CreatedAt time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt time.Time        `db:"updated_at" json:"updated_at"`
}

func (Download) TableName() string {
	return "downloads"
}

func init() {
	registerREPLModel("Download", Download{})
}
