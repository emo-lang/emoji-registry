package users_api

import (
	"net/http"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/errors"
	"github.com/emo-lang/emoji-registry/app/views/users"
)

// Routes registers the public user profile page at /u/:username.
func Routes(r *gin.Engine) {
	r.GET("/u/:username", ProfileAction)
}

// ProfileAction handles GET /u/:username — a user's public profile, listing
// the packages the account has published. Private packages are shown only to
// visitors who may read them.
func ProfileAction(c *gin.Context) {
	viewer := middlewares.CurrentUser(c)
	viewerName, admin := "", false
	if viewer != nil {
		viewerName, admin = viewer.Username, viewer.Admin
	}

	handle := c.Param("username")

	profileUser, err := repo.FindOneBy[models.User](sql.H{"username": handle})
	if err != nil {
		render.Error(c, err)
		return
	}
	if profileUser == nil {
		render.HTMLStatus(c, http.StatusNotFound, errors.NotFound(viewerName, "User "+handle+" does not exist."))
		return
	}

	pkgs, err := repo.Find[models.Package](repo.CurrentDB(),
		sql.All(models.Package{}).Where(sql.Eq("user_id", profileUser.ID)).OrderBy("created_at DESC"))
	if err != nil {
		render.Error(c, err)
		return
	}

	visible := pkgs[:0]
	for _, pkg := range pkgs {
		if middlewares.CanReadPackage(c, pkg) {
			visible = append(visible, pkg)
		}
	}

	render.HTML(c, users.Profile(viewerName, admin, profileUser, visible))
}
