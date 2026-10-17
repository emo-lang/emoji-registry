package admin_api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/admin"
)

// AccountsAction handles GET /admin/accounts — every user account and
// organization, newest first, each with its lock state and package count.
func AccountsAction(c *gin.Context) {
	username, adminFlag := adminViewer(c)

	users, err := repo.Find[models.User](repo.CurrentDB(), sql.All(models.User{}))
	if err != nil {
		render.Error(c, err)
		return
	}
	organizations, err := repo.Find[models.Organization](repo.CurrentDB(), sql.All(models.Organization{}))
	if err != nil {
		render.Error(c, err)
		return
	}

	counts, err := packageCountsByScope()
	if err != nil {
		render.Error(c, err)
		return
	}

	rows := make([]admin.AccountRow, 0, len(users)+len(organizations))
	for _, user := range users {
		rows = append(rows, admin.AccountRow{
			Kind:      "user",
			Name:      user.Username,
			URL:       "/u/" + user.Username,
			Packages:  counts[user.Username],
			Locked:    user.Locked,
			CreatedAt: user.CreatedAt,
			Path:      accountTogglePath("users", user.ID, user.Locked),
		})
	}
	for _, org := range organizations {
		rows = append(rows, admin.AccountRow{
			Kind:      "organization",
			Name:      org.Name,
			URL:       "/orgs/" + org.Name,
			Packages:  counts[org.Name],
			Locked:    org.Locked,
			CreatedAt: org.CreatedAt,
			Path:      accountTogglePath("orgs", org.ID, org.Locked),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})

	render.HTML(c, admin.Accounts(username, adminFlag, rows))
}

// LockAccountAction handles POST /admin/accounts/:kind/:id/lock — freezes
// an account or organization: its packages stay listed everywhere but
// cannot be downloaded, resolved or modified until unlocked.
func LockAccountAction(c *gin.Context) {
	setAccountLocked(c, true)
}

// UnlockAccountAction handles POST /admin/accounts/:kind/:id/unlock — the
// reversible half of the lock.
func UnlockAccountAction(c *gin.Context) {
	setAccountLocked(c, false)
}

// setAccountLocked flips the lock flag on the :kind/:id account. kind is
// "users" or "orgs" — the two lockable owner kinds.
func setAccountLocked(c *gin.Context, locked bool) {
	kind := c.Param("kind")

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		notFound(c)
		return
	}

	switch kind {
	case "users":
		err = repo.UpdateByID[models.User](sql.IdType(id), sql.H{"locked": locked})
	case "orgs":
		err = repo.UpdateByID[models.Organization](sql.IdType(id), sql.H{"locked": locked})
	default:
		notFound(c)
		return
	}
	if err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/admin/accounts")
}

// accountTogglePath builds the POST target that flips the account's current
// lock state: .../lock while active, .../unlock while locked.
func accountTogglePath(kind string, id sql.IdType, locked bool) string {
	action := "lock"
	if locked {
		action = "unlock"
	}
	return "/admin/accounts/" + kind + "/" + strconv.FormatInt(int64(id), 10) + "/" + action
}

// packageCountsByScope maps owner scopes to their package totals for the
// accounts list — one query instead of one per row.
func packageCountsByScope() (map[string]int, error) {
	pkgs, err := repo.FindAll[models.Package]()
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(pkgs))
	for _, pkg := range pkgs {
		counts[pkg.OwnerScope]++
	}
	return counts, nil
}
