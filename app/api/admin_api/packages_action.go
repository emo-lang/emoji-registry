package admin_api

import (
	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/admin"
)

// PackagesAction handles GET /admin — every published package, private
// ones included, newest first (id as the tie-breaker for identical
// timestamps).
func PackagesAction(c *gin.Context) {
	username, adminFlag := adminViewer(c)

	pkgs, err := repo.Find[models.Package](repo.CurrentDB(),
		sql.All(models.Package{}).OrderBy("created_at DESC").OrderBy("id DESC"))
	if err != nil {
		render.Error(c, err)
		return
	}

	counts, err := versionCounts()
	if err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, admin.Packages(username, adminFlag, pkgs, counts))
}
