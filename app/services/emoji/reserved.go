package emoji

import (
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

// reservedNames are top-level short names held back for the official Emo
// stdlib; packages under these owner scopes cannot be registered by ordinary
// accounts. This hardcoded list doubles as the compiler-side constant and as
// the seed data for the reserved_names table.
var reservedNames = map[string]bool{
	"net":    true,
	"http":   true,
	"core":   true,
	"std":    true,
	"sys":    true,
	"io":     true,
	"os":     true,
	"log":    true,
	"time":   true,
	"math":   true,
	"json":   true,
	"http2":  true,
	"ws":     true,
	"db":     true,
	"cache":  true,
	"queue":  true,
	"mail":   true,
	"test":   true,
	"debug":  true,
	"unsafe": true,
}

// ReservedSeedReason is stored on rows seeded from the hardcoded list.
const ReservedSeedReason = "official stdlib"

// IsReserved reports whether name is reserved: the union of the hardcoded
// stdlib list and the reserved_names table. When no database is set up (unit
// tests, tooling) only the hardcoded list applies.
func IsReserved(name string) bool {
	if reservedNames[name] {
		return true
	}

	if _, ok := repo.CurrentDBOK(); !ok {
		return false
	}

	exists, err := repo.ExistsWhere[models.ReservedName](sql.H{"name": name})
	return err == nil && exists
}

// SeedReservedNames inserts the hardcoded stdlib list into reserved_names.
// Existing rows are left untouched, so it is safe to call on every boot.
func SeedReservedNames() error {
	for name := range reservedNames {
		exists, err := repo.ExistsWhere[models.ReservedName](sql.H{"name": name})
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		if _, err := repo.CreateFrom[models.ReservedName](sql.H{
			"name":   name,
			"reason": ReservedSeedReason,
		}); err != nil {
			return err
		}
	}

	return nil
}
