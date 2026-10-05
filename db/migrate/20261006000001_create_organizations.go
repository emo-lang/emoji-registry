package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000001", "create_organizations", func(m *schema.Migrator) {
		m.CreateTable("organizations", func(t *schema.Table) {
			t.ID()
			t.String("name", 64).Null(false)
			t.String("display_name", 255).Null(false)
			t.References("users")
			t.Timestamps()
		})
		m.AddIndex("organizations", "name").Unique()
	})
}
