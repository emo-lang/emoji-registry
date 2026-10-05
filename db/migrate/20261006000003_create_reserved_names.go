package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000003", "create_reserved_names", func(m *schema.Migrator) {
		m.CreateTable("reserved_names", func(t *schema.Table) {
			t.ID()
			t.String("name", 64).Null(false)
			t.String("reason", 255).Null(false)
			t.Timestamps()
		})
		m.AddIndex("reserved_names", "name").Unique()
	})
}
