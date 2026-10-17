// Package lock reports the admin-imposed lock state of user accounts and
// organizations. A locked owner scope keeps its packages listed everywhere,
// but the registry refuses to serve their content: archives, version lists
// used by resolution, per-file reads and every write under the scope answer
// with the shared owner_locked error.
package lock

import (
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

// The stable error code and message every locked-scope refusal carries. The
// message is the text the CLI surfaces to the user during resolution, so it
// is final API surface, not copy to polish.
const (
	Code    = "owner_locked"
	Message = "该账号(组织）已被锁定"
)

// ScopeLocked reports whether the owner scope — a username or an
// organization name, the two sharing one namespace — belongs to a locked
// account. Unknown scopes are simply not locked.
func ScopeLocked(scope string) (bool, error) {
	user, err := repo.FindOneBy[models.User](sql.H{"username": scope})
	if err != nil {
		return false, err
	}
	if user != nil {
		return user.Locked, nil
	}

	org, err := repo.FindOneBy[models.Organization](sql.H{"name": scope})
	if err != nil {
		return false, err
	}
	return org != nil && org.Locked, nil
}
