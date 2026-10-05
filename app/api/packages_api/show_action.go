package packages_api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/views/errors"
	"github.com/emo-lang/emoji-registry/app/views/packages"
)

// ShowAction handles GET /p/:owner/:name — the public package detail page.
func ShowAction(c *gin.Context) {
	username := ""
	if user := middlewares.CurrentUser(c); user != nil {
		username = user.Username
	}

	owner := c.Param("owner")
	name := c.Param("name")

	notFound := func() {
		render.HTMLStatus(c, http.StatusNotFound, errors.NotFound(username, "Package "+owner+"/"+name+" does not exist."))
	}

	if !emoji.ValidNamePart(owner) || !emoji.ValidNamePart(name) {
		notFound()
		return
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
	if err != nil {
		render.Error(c, err)
		return
	}
	if pkg == nil {
		notFound()
		return
	}

	versions, err := repo.FindBy[models.Version](sql.H{"package_id": pkg.ID})
	if err != nil {
		render.Error(c, err)
		return
	}

	// Newest version first on the page.
	sort.Slice(versions, func(i, j int) bool {
		return emoji.CompareVersions(versions[i].Version, versions[j].Version) > 0
	})

	var latest *models.Version
	for _, version := range versions {
		if !version.Yanked() {
			latest = version
			break
		}
	}

	latestDeps := map[string]string{}
	if latest != nil && latest.Deps != "" {
		_ = json.Unmarshal([]byte(latest.Deps), &latestDeps)
	}

	render.HTML(c, packages.Show(username, pkg, versions, latest, latestDeps))
}
