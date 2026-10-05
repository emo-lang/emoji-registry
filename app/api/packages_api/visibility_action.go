package packages_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
	"github.com/emo-lang/emoji-registry/app/views/errors"
)

// SetVisibilityAction handles POST /p/:owner/:name/visibility — the package
// page's public/private toggle (web session, owning scope members only).
func SetVisibilityAction(c *gin.Context) {
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

	visibility := strings.TrimSpace(c.PostForm("visibility"))
	if visibility != models.VisibilityPublic && visibility != models.VisibilityPrivate {
		c.Redirect(http.StatusFound, "/p/"+pkg.FullName())
		return
	}

	if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{"visibility": visibility}); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/p/"+pkg.FullName())
}
