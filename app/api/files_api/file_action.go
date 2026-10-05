package files_api

import (
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// VersionsAction serves GET /:owner/:name/versions: a JSON array of version
// strings in ascending semver order, yanked versions excluded.
func VersionsAction(c *gin.Context) {
	owner := c.Param("owner")
	name := c.Param("name")

	if !emoji.ValidNamePart(owner) || !emoji.ValidNamePart(name) {
		respondNotFound(c)
		return
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
	if err != nil {
		respondNotFound(c)
		return
	}
	if pkg == nil {
		respondNotFound(c)
		return
	}

	versions, err := repo.FindBy[models.Version](sql.H{"package_id": pkg.ID})
	if err != nil {
		respondNotFound(c)
		return
	}

	sorted := make([]*models.Version, 0, len(versions))
	for _, version := range versions {
		if !version.Yanked() {
			sorted = append(sorted, version)
		}
	}
	sortVersions(sorted)

	list := make([]string, 0, len(sorted))
	for _, version := range sorted {
		list = append(list, version.Version)
	}

	c.JSON(http.StatusOK, list)
}

// FileAction serves GET /:owner/:name/:version/<path>.emo, including the
// package.emo manifest. Only .emo sources are reachable; yanked versions
// answer 404 because protocol A feeds dependency resolution.
func FileAction(c *gin.Context) {
	owner := c.Param("owner")
	name := c.Param("name")
	version := c.Param("version")
	filepath := strings.TrimPrefix(c.Param("filepath"), "/")

	if !emoji.ValidNamePart(owner) || !emoji.ValidNamePart(name) || !emoji.ValidVersion(version) {
		respondNotFound(c)
		return
	}

	cleaned := path.Clean(filepath)
	if cleaned != filepath || cleaned == "." || path.IsAbs(cleaned) ||
		cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
		!strings.HasSuffix(cleaned, ".emo") {
		respondNotFound(c)
		return
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
	if err != nil || pkg == nil {
		respondNotFound(c)
		return
	}

	row, err := repo.FindOneBy[models.Version](sql.H{"package_id": pkg.ID, "version": version})
	if err != nil || row == nil || row.Yanked() {
		respondNotFound(c)
		return
	}

	key := "packages/" + owner + "/" + name + "/" + version + "/files/" + cleaned

	r, err := storage.Current().Get(c.Request.Context(), key)
	if err != nil {
		respondNotFound(c)
		return
	}
	defer r.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, r)
}

func respondNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "not found"}})
}
