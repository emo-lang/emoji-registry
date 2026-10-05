package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261006000002", "create_memberships", func(m *schema.Migrator) {
		m.CreateTable("memberships", func(t *schema.Table) {
			t.ID()
			t.References("organizations")
			t.References("users")
			t.String("role", 16).Null(false)
			t.Timestamps()
		})
		m.AddIndex("memberships", "organization_id", "user_id").Unique()
	})
}
