package registry_api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DependenciesAction handles GET /api/v1/dependencies?packages=a/b,c/d (B.2).
// Unknown packages are absent from the response.
func DependenciesAction(c *gin.Context) {
	query := c.Query("packages")

	result := gin.H{}
	if strings.TrimSpace(query) == "" {
		c.JSON(http.StatusOK, result)
		return
	}

	for _, fullName := range strings.Split(query, ",") {
		owner, name, found := strings.Cut(strings.TrimSpace(fullName), "/")
		if !found || owner == "" || name == "" {
			continue
		}

		pkg, err := findPackage(owner, name)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if pkg == nil {
			continue
		}

		versions, err := packageVersions(pkg)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		list := make([]gin.H, 0, len(versions))
		for _, version := range versions {
			list = append(list, gin.H{
				"version": version.Version,
				"deps":    versionDeps(version),
				"yanked":  version.Yanked(),
			})
		}

		result[pkg.FullName()] = list
	}

	c.JSON(http.StatusOK, result)
}
