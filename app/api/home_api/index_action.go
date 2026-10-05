package home_api

import (
	"sort"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/home"
)

// IndexAction renders the registry home page: newest and most downloaded
// packages.
func IndexAction(c *gin.Context) {
	pkgs, err := repo.FindAll[models.Package]()
	if err != nil {
		render.Error(c, err)
		return
	}

	latest := make([]*models.Package, len(pkgs))
	copy(latest, pkgs)
	sort.Slice(latest, func(i, j int) bool {
		return latest[i].CreatedAt.After(latest[j].CreatedAt)
	})
	if len(latest) > 10 {
		latest = latest[:10]
	}

	top := make([]*models.Package, len(pkgs))
	copy(top, pkgs)
	sort.Slice(top, func(i, j int) bool {
		return top[i].Downloads > top[j].Downloads
	})
	if len(top) > 10 {
		top = top[:10]
	}

	render.HTML(c, home.Index(currentUsername(c), latest, top))
}

func currentUsername(c *gin.Context) string {
	if user := middlewares.CurrentUser(c); user != nil {
		return user.Username
	}
	return ""
}
