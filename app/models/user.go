package models

import (
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

type User struct {
	ID             airwaysql.IdType `db:"id" json:"id"`
	Username       string           `db:"username" json:"username"`
	Email          string           `db:"email" json:"email"`
	PasswordDigest string           `db:"password_digest" json:"-"`
	Admin          bool             `db:"admin" json:"admin"`
	CreatedAt      time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time        `db:"updated_at" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

func init() {
	registerREPLModel("User", User{})
}
