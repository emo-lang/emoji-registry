package admin

import (
	"strconv"
	"time"

	airwaysql "github.com/daqing/airway/lib/sql"
)

// AccountRow is one row of the admin accounts list: a user account or an
// organization flattened to what the table shows. Path is the POST target
// that flips the current lock state; Reset2FAPath is empty for organizations.
type AccountRow struct {
	Kind         string // "user" or "organization", display only
	Name         string
	URL          string
	Packages     int
	Locked       bool
	TwoFA        bool
	CreatedAt    time.Time
	Path         string
	Reset2FAPath string
}

func idString(id airwaysql.IdType) string {
	return strconv.FormatInt(int64(id), 10)
}
