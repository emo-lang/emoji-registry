package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000004", "create_downloads", func(m *schema.Migrator) {
		m.CreateTable("downloads", func(t *schema.Table) {
			t.ID()
			t.References("versions")
			t.String("date", 10).Null(false)
			t.BigInt("count").Null(false).Default(0)
			t.Timestamps()
		})
		m.AddIndex("downloads", "version_id", "date").Unique()
	})
}
