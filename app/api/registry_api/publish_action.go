package registry_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// maxUploadSize bounds the raw .emoji upload body; the uncompressed content
// limit is enforced separately inside emoji.Validate.
const maxUploadSize = 20 << 20 // 20 MB

// PublishAction handles POST /api/v1/packages: the body is the raw .emoji
// archive and name/version come only from its package.emo manifest.
func PublishAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize))
	if err != nil {
		respondError(c, http.StatusRequestEntityTooLarge, "invalid_manifest", "archive exceeds the upload size limit")
		return
	}

	result, err := emoji.ProcessUpload(body)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_manifest", err.Error())
		return
	}

	owner := result.Manifest.Owner()
	name := result.Manifest.ShortName()

	if emoji.IsReserved(owner) {
		respondError(c, http.StatusForbidden, "name_reserved", owner+" is reserved for the official stdlib")
		return
	}
	if owner != user.Username {
		respondError(c, http.StatusForbidden, "forbidden", "you can only publish under your own scope "+user.Username)
		return
	}

	pkg, err := findPackage(owner, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if pkg == nil {
		pkg, err = repo.CreateFrom[models.Package](sql.H{
			"owner_scope": owner,
			"name":        name,
			"description": "",
			"license":     "",
			"homepage":    "",
			"repository":  "",
			"user_id":     user.ID,
			"downloads":   0,
		})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else if pkg.UserID != user.ID {
		respondError(c, http.StatusForbidden, "forbidden", "you do not own "+pkg.FullName())
		return
	}

	version := result.Manifest.Version

	exists, err := repo.ExistsWhere[models.Version](sql.H{"package_id": pkg.ID, "version": version})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if exists {
		respondError(c, http.StatusConflict, "version_exists", pkg.FullName()+" "+version+" already published")
		return
	}

	if err := storeArchive(c, pkg, version, result); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	deps, err := json.Marshal(result.Manifest.Deps)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	storageKey := archiveStorageKey(pkg, version)

	row, err := repo.CreateFrom[models.Version](sql.H{
		"package_id":     pkg.ID,
		"version":        version,
		"checksum":       result.Digest,
		"archive_sha256": result.ArchiveSHA256,
		"targets":        strings.Join(result.Manifest.Targets, ","),
		"deps":           string(deps),
		"size":           int64(len(result.Archive)),
		"storage_key":    storageKey,
		"user_id":        user.ID,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusCreated, versionObject(row))
}

func archiveStorageKey(pkg *models.Package, version string) string {
	return fmt.Sprintf("packages/%s/%s/%s/package.emoji", pkg.OwnerScope, pkg.Name, version)
}

// storeArchive persists the canonical archive and each individual .emo source
// (protocol A serves the per-file keys).
func storeArchive(c *gin.Context, pkg *models.Package, version string, result *emoji.Result) error {
	ctx := c.Request.Context()
	store := storage.Current()

	if err := store.Put(ctx, archiveStorageKey(pkg, version), storage.Object{
		Reader:      bytes.NewReader(result.Archive),
		Size:        int64(len(result.Archive)),
		ContentType: "application/gzip",
	}); err != nil {
		return err
	}

	prefix := fmt.Sprintf("packages/%s/%s/%s/files/", pkg.OwnerScope, pkg.Name, version)
	for path, content := range result.Files {
		if err := store.Put(ctx, prefix+path, storage.Object{
			Reader:      bytes.NewReader(content),
			Size:        int64(len(content)),
			ContentType: "text/plain; charset=utf-8",
		}); err != nil {
			return err
		}
	}

	return nil
}
