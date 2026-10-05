package migrations

import (
	"github.com/daqing/airway/lib/migrate/schema"
)

func init() {
	schema.RegisterChange("20261005000005", "create_versions", func(m *schema.Migrator) {
		m.CreateTable("versions", func(t *schema.Table) {
			t.ID()
			t.References("packages")
			t.String("version", 32).Null(false)
			t.String("checksum", 64).Null(false)
			t.String("archive_sha256", 64).Null(false)
			t.String("targets", 255).Null(false)
			t.JSON("deps")
			t.BigInt("size").Null(false).Default(0)
			t.String("storage_key", 512).Null(false)
			t.DateTime("yanked_at")
			t.References("users")
			t.Timestamps()
		})
		m.AddIndex("versions", "package_id", "version").Unique()
	})
}
