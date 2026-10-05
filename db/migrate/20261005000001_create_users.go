package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261005000001", "create_users", func(m *schema.Migrator) {
		m.CreateTable("users", func(t *schema.Table) {
			t.ID()
			t.String("username", 64).Null(false)
			t.String("email", 255).Null(false)
			t.String("password_digest", 255).Null(false)
			t.Timestamps()
		})
		m.AddIndex("users", "username").Unique()
		m.AddIndex("users", "email").Unique()
	})
}
