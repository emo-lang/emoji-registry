package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261018000003", "create_totp_recovery_codes", func(m *schema.Migrator) {
		m.CreateTable("totp_recovery_codes", func(t *schema.Table) {
			t.ID()
			t.References("users")
			t.String("code_hash", 64).Null(false)
			t.DateTime("used_at")
			t.Timestamps()
		})
		m.AddIndex("totp_recovery_codes", "user_id", "code_hash").Unique()
	})
}
