package home_api

import (
	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/home"
)

// IndexAction renders the registry home page: newest and most downloaded
// packages.
func IndexAction(c *gin.Context) {
	latest, err := repo.Find[models.Package](repo.CurrentDB(),
		sql.All(models.Package{}).Where(sql.Eq("visibility", models.VisibilityPublic)).OrderBy("created_at DESC").Limit(10))
	if err != nil {
		render.Error(c, err)
		return
	}

	top, err := repo.Find[models.Package](repo.CurrentDB(),
		sql.All(models.Package{}).Where(sql.Eq("visibility", models.VisibilityPublic)).OrderBy("downloads DESC").Limit(10))
	if err != nil {
		render.Error(c, err)
		return
	}

	username, admin := "", false
	if user := middlewares.CurrentUser(c); user != nil {
		username, admin = user.Username, user.Admin
	}

	render.HTML(c, home.Index(username, admin, latest, top))
}
