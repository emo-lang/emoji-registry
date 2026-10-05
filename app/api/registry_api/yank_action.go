package registry_api

import (
	"net/http"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// YankAction handles DELETE /api/v1/packages/:owner/:name/versions/:version.
// Yanking is a soft delete and idempotent: re-yanking returns 200.
func YankAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	pkg, ok := loadPackageForRequest(c)
	if !ok {
		return
	}

	if !orgs.CanPublishAs(user, pkg.OwnerScope) {
		respondError(c, http.StatusForbidden, "forbidden", "only a member of the owning scope can yank "+pkg.FullName())
		return
	}

	version, err := repo.FindOneBy[models.Version](sql.H{
		"package_id": pkg.ID,
		"version":    c.Param("version"),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if version == nil {
		respondError(c, http.StatusNotFound, "version_not_found", pkg.FullName()+" "+c.Param("version")+" is not published")
		return
	}

	if !version.Yanked() {
		now := time.Now()
		if err := repo.UpdateByID[models.Version](version.ID, sql.H{"yanked_at": now}); err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		version.YankedAt = &now
	}

	c.JSON(http.StatusOK, versionObject(version))
}
