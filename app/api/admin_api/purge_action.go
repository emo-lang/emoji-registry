package admin_api

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// PurgePhrase prefixes the exact sentence an admin must type to force-delete
// a package, e.g. "I want to purge this package david/foo-bar".
const PurgePhrase = "I want to purge this package "

// PurgeAction handles POST /admin/packages/:owner/:name/purge — the
// force-delete behind the admin list's confirmation dialog. The typed phrase
// must match PurgePhrase+FullName exactly; the browser-side check is only
// convenience, this is the gate.
func PurgeAction(c *gin.Context) {
	pkg, err := repo.FindOneBy[models.Package](sql.H{
		"owner_scope": c.Param("owner"),
		"name":        c.Param("name"),
	})
	if err != nil {
		render.Error(c, err)
		return
	}
	if pkg == nil {
		notFound(c)
		return
	}

	if strings.TrimSpace(c.PostForm("confirm")) != PurgePhrase+pkg.FullName() {
		renderPackages(c, "The confirmation phrase did not match; nothing was deleted.", http.StatusUnprocessableEntity)
		return
	}

	if err := purgePackage(c.Request.Context(), pkg); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/admin")
}

// purgePackage removes every trace of pkg: the stored objects first (their
// content is not recoverable from the database), then the rows in dependency
// order — download aggregates, versions, the package itself.
func purgePackage(ctx context.Context, pkg *models.Package) error {
	versions, err := repo.FindBy[models.Version](sql.H{"package_id": pkg.ID})
	if err != nil {
		return err
	}

	store := storage.Current()
	for _, v := range versions {
		if err := purgeVersionFiles(ctx, store, v); err != nil {
			return err
		}
	}

	for _, v := range versions {
		if err := repo.DeleteWhere[models.Download](sql.H{"version_id": v.ID}); err != nil {
			return err
		}
	}
	if err := repo.DeleteWhere[models.Version](sql.H{"package_id": pkg.ID}); err != nil {
		return err
	}
	return repo.DeleteByID[models.Package](pkg.ID)
}

// purgeVersionFiles deletes the archive and every per-file key the publish
// flow wrote. The storage layer has no listing, so the file names come out of
// the stored archive itself — it is the canonical content. A missing or
// unreadable archive only means its files cannot be enumerated; the known
// keys are still deleted, and Delete treats missing objects as success.
func purgeVersionFiles(ctx context.Context, store storage.Storage, v *models.Version) error {
	if v.StorageKey == "" {
		return nil
	}

	files := map[string][]byte{}
	if r, err := store.Get(ctx, v.StorageKey); err == nil {
		if data, err := io.ReadAll(r); err == nil {
			files, _ = emoji.Unpack(data)
		}
		r.Close()
	}

	prefix := strings.TrimSuffix(v.StorageKey, "/package.emoji")
	for name := range files {
		if err := store.Delete(ctx, prefix+"/files/"+name); err != nil {
			return err
		}
	}
	if err := store.Delete(ctx, v.StorageKey); err != nil {
		return err
	}

	// Strip the now-empty directories behind the keys. The local backend's
	// os.Remove only succeeds on empty dirs; cloud backends have no dir
	// objects and answer NotFound, which Delete ignores — so failures here
	// mean "not empty yet" (more versions live under the package dir) and
	// are not an error.
	_ = store.Delete(ctx, prefix+"/files")
	_ = store.Delete(ctx, prefix)
	_ = store.Delete(ctx, path.Dir(prefix))
	return nil
}
