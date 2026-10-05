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
		sql.All(models.Package{}).OrderBy("created_at DESC").Limit(10))
	if err != nil {
		render.Error(c, err)
		return
	}

	top, err := repo.Find[models.Package](repo.CurrentDB(),
		sql.All(models.Package{}).OrderBy("downloads DESC").Limit(10))
	if err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, home.Index(currentUsername(c), latest, top))
}

func currentUsername(c *gin.Context) string {
	if user := middlewares.CurrentUser(c); user != nil {
		return user.Username
	}
	return ""
}
