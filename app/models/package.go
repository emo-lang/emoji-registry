package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type Package struct {
	ID          airwaysql.IdType `db:"id" json:"id"`
	OwnerScope  string           `db:"owner_scope" json:"owner_scope"`
	Name        string           `db:"name" json:"name"`
	Description string           `db:"description" json:"description"`
	License     string           `db:"license" json:"license"`
	Homepage    string           `db:"homepage" json:"homepage"`
	Repository  string           `db:"repository" json:"repository"`
	UserID      airwaysql.IdType `db:"user_id" json:"user_id"`
	Downloads   int64            `db:"downloads" json:"downloads"`
	Visibility  string           `db:"visibility" json:"visibility"`
	CreatedAt   time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time        `db:"updated_at" json:"updated_at"`
}

// Package visibility values.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func (p *Package) Private() bool {
	return p.Visibility == VisibilityPrivate
}

func (Package) TableName() string {
	return "packages"
}

func (p *Package) FullName() string {
	return p.OwnerScope + "/" + p.Name
}

func init() {
	registerREPLModel("Package", Package{})
}
