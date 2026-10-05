package registry_api

import (
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// ShowPackageAction handles GET /api/v1/packages/:owner/:name (B.4).
func ShowPackageAction(c *gin.Context) {
	pkg, ok := loadPackageForRequest(c)
	if !ok {
		return
	}

	versions, err := packageVersions(pkg)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, packageObject(pkg, versions))
}

// UpdatePackageAction handles PATCH /api/v1/packages/:owner/:name. Only the
// package owner may edit the server-side metadata fields.
func UpdatePackageAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	pkg, ok := loadPackageForRequest(c)
	if !ok {
		return
	}

	if !orgs.CanPublishAs(user, pkg.OwnerScope) {
		respondError(c, http.StatusForbidden, "forbidden", "only a member of the owning scope can edit "+pkg.FullName())
		return
	}

	var input struct {
		Description *string `json:"description"`
		License     *string `json:"license"`
		Homepage    *string `json:"homepage"`
		Repository  *string `json:"repository"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with description/license/homepage/repository")
		return
	}

	updates := sql.H{"updated_at": time.Now()}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
	}
	if input.License != nil {
		updates["license"] = strings.TrimSpace(*input.License)
	}
	if input.Homepage != nil {
		updates["homepage"] = strings.TrimSpace(*input.Homepage)
	}
	if input.Repository != nil {
		updates["repository"] = strings.TrimSpace(*input.Repository)
	}

	if err := repo.UpdateByID[models.Package](pkg.ID, updates); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	updated, err := repo.FindByID[models.Package](pkg.ID)
	if err != nil || updated == nil {
		respondError(c, http.StatusInternalServerError, "internal_error", "failed to reload package")
		return
	}

	versions, err := packageVersions(updated)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, packageObject(updated, versions))
}
