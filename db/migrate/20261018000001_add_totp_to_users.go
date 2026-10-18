package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261018000001", "add_totp_to_users", func(m *schema.Migrator) {
		m.AddColumn("users", schema.Column{
			Name:    "totp_secret",
			Type:    schema.Type{Kind: schema.TypeString, Length: 64},
			Null:    schema.Bool(false),
			Default: "",
		})
		m.AddColumn("users", schema.Column{
			Name:    "totp_enabled",
			Type:    schema.Type{Kind: schema.TypeBoolean},
			Null:    schema.Bool(false),
			Default: false,
		})
	})
}
