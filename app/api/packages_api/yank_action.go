package packages_api

import (
	"net/http"
	"time"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
	"github.com/emo-lang/emoji-registry/app/views/errors"
)

// YankVersionAction handles POST /p/:owner/:name/versions/:version/yank — the
// package page's per-version yank button (web session, owning scope members
// only). Yanking is a soft delete and idempotent; the same effect as
// DELETE /api/v1/packages/:owner/:name/versions/:version.
func YankVersionAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	pkg, err := repo.FindOneBy[models.Package](sql.H{
		"owner_scope": c.Param("owner"),
		"name":        c.Param("name"),
	})
	if err != nil {
		render.Error(c, err)
		return
	}
	if pkg == nil || !orgs.CanPublishAs(user, pkg.OwnerScope) {
		render.HTMLStatus(c, http.StatusNotFound,
			errors.NotFound(user.Username, "Package "+c.Param("owner")+"/"+c.Param("name")+" does not exist."))
		return
	}

	version, err := repo.FindOneBy[models.Version](sql.H{
		"package_id": pkg.ID,
		"version":    c.Param("version"),
	})
	if err != nil {
		render.Error(c, err)
		return
	}
	if version == nil {
		c.Redirect(http.StatusFound, "/p/"+pkg.FullName())
		return
	}

	if !version.Yanked() {
		now := time.Now()
		if err := repo.UpdateByID[models.Version](version.ID, sql.H{"yanked_at": now}); err != nil {
			render.Error(c, err)
			return
		}
	}

	c.Redirect(http.StatusFound, "/p/"+pkg.FullName())
}
