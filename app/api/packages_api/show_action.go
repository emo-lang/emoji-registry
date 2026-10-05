package packages_api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/a-h/templ"
	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/markdown"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
	"github.com/emo-lang/emoji-registry/app/services/stats"
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

	versionIDs := make([]sql.IdType, 0, len(versions))
	for _, v := range versions {
		versionIDs = append(versionIDs, v.ID)
	}

	daily, err := stats.DailySeries(versionIDs, 30, time.Now())
	if err != nil {
		render.Error(c, err)
		return
	}

	ownerIsOrg := false
	if org, err := orgs.FindByName(pkg.OwnerScope); err == nil && org != nil {
		ownerIsOrg = true
	}

	var readme templ.Component
	if latest != nil {
		readme = loadReadme(c, pkg, latest.Version)
	}

	render.HTML(c, packages.Show(username, pkg, versions, latest, latestDeps, ownerIsOrg, daily, readme))
}

// loadReadme fetches and renders the README.md stored next to the version's
// sources; nil when the version shipped none.
func loadReadme(c *gin.Context, pkg *models.Package, version string) templ.Component {
	key := fmt.Sprintf("packages/%s/%s/%s/files/%s", pkg.OwnerScope, pkg.Name, version, emoji.ReadmeFileName)

	r, err := storage.Current().Get(c.Request.Context(), key)
	if err != nil {
		return nil
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil
	}

	html, err := markdown.Render(content)
	if err != nil {
		return nil
	}

	return templ.Raw(html)
}
