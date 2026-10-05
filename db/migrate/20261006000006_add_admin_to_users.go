package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000006", "add_admin_to_users", func(m *schema.Migrator) {
		m.AddColumn("users", schema.Column{
			Name:    "admin",
			Type:    schema.Type{Kind: schema.TypeBoolean},
			Null:    schema.Bool(false),
			Default: false,
		})
	})
}
