package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000005", "add_visibility_to_packages", func(m *schema.Migrator) {
		m.AddColumn("packages", schema.Column{
			Name:    "visibility",
			Type:    schema.Type{Kind: schema.TypeString, Length: 16},
			Null:    schema.Bool(false),
			Default: "public",
		})
	})
}
