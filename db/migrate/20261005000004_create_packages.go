package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261005000004", "create_packages", func(m *schema.Migrator) {
		m.CreateTable("packages", func(t *schema.Table) {
			t.ID()
			t.String("owner_scope", 64).Null(false)
			t.String("name", 64).Null(false)
			t.Text("description")
			t.String("license", 64)
			t.String("homepage", 255)
			t.String("repository", 255)
			t.References("users")
			t.BigInt("downloads").Null(false).Default(0)
			t.Timestamps()
		})
		m.AddIndex("packages", "owner_scope", "name").Unique()
	})
}
