package downloads_api

import (
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/stats"
)

// DownloadAction serves GET /downloads/<owner>--<name>--<version>.emoji.
// Yanked versions stay downloadable (lockfile pins must keep working) and are
// flagged with X-Emo-Yanked.
func DownloadAction(c *gin.Context) {
	owner, name, version, ok := parseFileName(c.Param("file"))
	if !ok {
		respondError(c, http.StatusNotFound, "package_not_found", "malformed download file name")
		return
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
	if err != nil || pkg == nil {
		respondError(c, http.StatusNotFound, "package_not_found", owner+"/"+name+" is not published")
		return
	}
	if !middlewares.CanReadPackage(c, pkg) {
		respondError(c, http.StatusNotFound, "package_not_found", owner+"/"+name+" is not published")
		return
	}

	row, err := repo.FindOneBy[models.Version](sql.H{"package_id": pkg.ID, "version": version})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if row == nil {
		respondError(c, http.StatusNotFound, "version_not_found", owner+"/"+name+" "+version+" is not published")
		return
	}

	r, err := storage.Current().Get(c.Request.Context(), row.StorageKey)
	if err != nil {
		respondError(c, http.StatusNotFound, "version_not_found", "archive is missing from storage")
		return
	}
	defer r.Close()

	digest := ""
	if raw, err := hex.DecodeString(row.ArchiveSHA256); err == nil {
		digest = "sha-256=" + base64.StdEncoding.EncodeToString(raw)
	}

	c.Header("Content-Type", "application/gzip")
	if digest != "" {
		c.Header("Digest", digest)
	}
	c.Header("ETag", `"`+row.ArchiveSHA256+`"`)
	if row.Yanked() {
		c.Header("X-Emo-Yanked", "true")
	}
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, r); err != nil {
		return
	}

	_ = stats.Record(pkg.ID, row.ID, time.Now())
}

// parseFileName splits <owner>--<name>--<version>.emoji into its parts.
func parseFileName(file string) (owner, name, version string, ok bool) {
	base, found := strings.CutSuffix(file, ".emoji")
	if !found {
		return "", "", "", false
	}

	parts := strings.Split(base, "--")
	if len(parts) != 3 {
		return "", "", "", false
	}

	owner, name, version = parts[0], parts[1], parts[2]
	if !emoji.ValidNamePart(owner) || !emoji.ValidNamePart(name) || !emoji.ValidVersion(version) {
		return "", "", "", false
	}

	return owner, name, version, true
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
