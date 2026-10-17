package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261017000001", "add_locked_to_owners", func(m *schema.Migrator) {
		m.AddColumn("users", schema.Column{
			Name:    "locked",
			Type:    schema.Type{Kind: schema.TypeBoolean},
			Null:    schema.Bool(false),
			Default: false,
		})
		m.AddColumn("organizations", schema.Column{
			Name:    "locked",
			Type:    schema.Type{Kind: schema.TypeBoolean},
			Null:    schema.Bool(false),
			Default: false,
		})
	})
}
