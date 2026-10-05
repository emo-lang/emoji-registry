package registry_api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/stats"
)

// respondError emits the shared error shape from docs/design.md section 5.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// versionObject renders the frozen B.1 version schema.
func versionObject(v *models.Version) gin.H {
	return gin.H{
		"version":        v.Version,
		"checksum":       v.Checksum,
		"archive_sha256": v.ArchiveSHA256,
		"targets":        v.TargetList(),
		"yanked":         v.Yanked(),
		"published_at":   v.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// versionDeps parses the stored JSON deps column back into a map.
func versionDeps(v *models.Version) map[string]string {
	deps := map[string]string{}
	if v.Deps != "" {
		_ = json.Unmarshal([]byte(v.Deps), &deps)
	}
	return deps
}

// findPackage looks up a package by scope and short name; nil when missing.
func findPackage(owner, name string) (*models.Package, error) {
	return repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
}

// packageVersions returns every version row of pkg ordered by ascending semver.
func packageVersions(pkg *models.Package) ([]*models.Version, error) {
	versions, err := repo.FindBy[models.Version](sql.H{"package_id": pkg.ID})
	if err != nil {
		return nil, err
	}

	sort.Slice(versions, func(i, j int) bool {
		return emoji.CompareVersions(versions[i].Version, versions[j].Version) < 0
	})

	return versions, nil
}

// latestVersion returns the highest non-yanked version, or "" when every
// version is yanked (or none exist).
func latestVersion(versions []*models.Version) string {
	for i := len(versions) - 1; i >= 0; i-- {
		if !versions[i].Yanked() {
			return versions[i].Version
		}
	}
	return ""
}

// packageObject renders the frozen B.4 package schema.
func packageObject(pkg *models.Package, versions []*models.Version) gin.H {
	versionIDs := make([]sql.IdType, 0, len(versions))
	for _, v := range versions {
		versionIDs = append(versionIDs, v.ID)
	}

	last30d, err := stats.SumSince(versionIDs, time.Now().AddDate(0, 0, -30).Format("2006-01-02"))
	if err != nil {
		last30d = 0
	}

	return gin.H{
		"name":               pkg.FullName(),
		"description":        pkg.Description,
		"license":            pkg.License,
		"homepage":           pkg.Homepage,
		"repository":         pkg.Repository,
		"downloads":          pkg.Downloads,
		"downloads_last_30d": last30d,
		"owners":             []string{pkg.OwnerScope},
		"latest_version":     latestVersion(versions),
		"created_at":         pkg.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         pkg.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// loadPackageForRequest resolves :owner/:name path params, answering with
// package_not_found when needed. The second return value reports whether the
// package was found.
func loadPackageForRequest(c *gin.Context) (*models.Package, bool) {
	owner := c.Param("owner")
	name := c.Param("name")

	pkg, err := findPackage(owner, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return nil, false
	}
	if pkg == nil {
		respondError(c, http.StatusNotFound, "package_not_found", owner+"/"+name+" is not published")
		return nil, false
	}

	return pkg, true
}
