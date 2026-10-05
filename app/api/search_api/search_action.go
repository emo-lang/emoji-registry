package search_api

import (
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/search"
)

// SearchAction handles GET /search?q=: a case-insensitive substring match
// over owner scope, name and description.
func SearchAction(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	results := []*models.Package{}

	if q != "" {
		pkgs, err := repo.FindAll[models.Package]()
		if err != nil {
			render.Error(c, err)
			return
		}

		needle := strings.ToLower(q)
		for _, pkg := range pkgs {
			if strings.Contains(strings.ToLower(pkg.OwnerScope), needle) ||
				strings.Contains(strings.ToLower(pkg.Name), needle) ||
				strings.Contains(strings.ToLower(pkg.Description), needle) {
				results = append(results, pkg)
			}
		}
	}

	username := ""
	if user := middlewares.CurrentUser(c); user != nil {
		username = user.Username
	}

	render.HTML(c, search.Results(username, q, results))
}
