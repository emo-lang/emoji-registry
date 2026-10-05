package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261005000002", "create_sessions", func(m *schema.Migrator) {
		m.CreateTable("sessions", func(t *schema.Table) {
			t.ID()
			t.String("token", 64).Null(false)
			t.References("users")
			t.DateTime("expires_at").Null(false)
			t.Timestamps()
		})
		m.AddIndex("sessions", "token").Unique()
	})
}
