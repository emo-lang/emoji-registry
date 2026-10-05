package models

import (
	"strings"
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type Version struct {
	ID            airwaysql.IdType `db:"id" json:"id"`
	PackageID     airwaysql.IdType `db:"package_id" json:"package_id"`
	Version       string           `db:"version" json:"version"`
	Checksum      string           `db:"checksum" json:"checksum"`
	ArchiveSHA256 string           `db:"archive_sha256" json:"archive_sha256"`
	Targets       string           `db:"targets" json:"targets"`
	Deps          string           `db:"deps" json:"deps"`
	Size          int64            `db:"size" json:"size"`
	StorageKey    string           `db:"storage_key" json:"storage_key"`
	YankedAt      *time.Time       `db:"yanked_at" json:"yanked_at"`
	UserID        airwaysql.IdType `db:"user_id" json:"user_id"`
	CreatedAt     time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time        `db:"updated_at" json:"updated_at"`
}

func (Version) TableName() string {
	return "versions"
}

func (v *Version) Yanked() bool {
	return v.YankedAt != nil
}

func (v *Version) TargetList() []string {
	if strings.TrimSpace(v.Targets) == "" {
		return []string{}
	}

	return strings.Split(v.Targets, ",")
}

func init() {
	registerREPLModel("Version", Version{})
}
