package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261018000002", "add_totp_pending_to_sessions", func(m *schema.Migrator) {
		m.AddColumn("sessions", schema.Column{
			Name:    "totp_pending",
			Type:    schema.Type{Kind: schema.TypeBoolean},
			Null:    schema.Bool(false),
			Default: false,
		})
	})
}
