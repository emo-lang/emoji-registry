package registry_api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListVersionsAction handles GET /api/v1/packages/:owner/:name/versions (B.1).
func ListVersionsAction(c *gin.Context) {
	pkg, ok := loadPackageForRequest(c)
	if !ok {
		return
	}

	versions, err := packageVersions(pkg)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	list := make([]gin.H, 0, len(versions))
	for _, version := range versions {
		list = append(list, versionObject(version))
	}

	c.JSON(http.StatusOK, gin.H{
		"package":  pkg.FullName(),
		"versions": list,
	})
}
