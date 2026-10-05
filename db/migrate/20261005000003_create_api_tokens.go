package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261005000003", "create_api_tokens", func(m *schema.Migrator) {
		m.CreateTable("api_tokens", func(t *schema.Table) {
			t.ID()
			t.References("users")
			t.String("name", 64).Null(false)
			t.String("token_hash", 64).Null(false)
			t.String("scopes", 255).Null(false)
			t.DateTime("expires_at")
			t.DateTime("last_used_at")
			t.Timestamps()
		})
		m.AddIndex("api_tokens", "token_hash").Unique()
	})
}
