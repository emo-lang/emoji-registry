package admin_api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// seedPublishedVersion mimics the publish flow for one version: the canonical
// archive at the archive key, the per-file keys under files/, and the
// database rows including a download aggregate.
func seedPublishedVersion(t *testing.T, pkg *models.Package, owner *models.User, version string) *models.Version {
	t.Helper()

	archive, err := emoji.BuildArchive(map[string][]byte{
		"package.emo":     []byte("package {\n  name = \"" + pkg.FullName() + "\"\n  version = \"" + version + "\"\n}\n"),
		pkg.Name + ".emo": []byte("let main = 1\n"),
		"README.md":       []byte("# " + pkg.FullName() + "\n"),
	})
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}

	store := storage.Current()
	ctx := context.Background()
	archiveKey := "packages/" + pkg.OwnerScope + "/" + pkg.Name + "/" + version + "/package.emoji"
	if err := store.Put(ctx, archiveKey, storage.Object{
		Reader: strings.NewReader(string(archive)), Size: int64(len(archive)), ContentType: "application/gzip",
	}); err != nil {
		t.Fatalf("put archive: %v", err)
	}

	filesPrefix := "packages/" + pkg.OwnerScope + "/" + pkg.Name + "/" + version + "/files/"
	for _, name := range []string{"package.emo", pkg.Name + ".emo", "README.md"} {
		if err := store.Put(ctx, filesPrefix+name, storage.Object{
			Reader: strings.NewReader("x"), Size: 1, ContentType: "text/plain; charset=utf-8",
		}); err != nil {
			t.Fatalf("put file %s: %v", name, err)
		}
	}

	row, err := repo.CreateFrom[models.Version](sql.H{
		"package_id": pkg.ID, "version": version,
		"checksum": "ab", "archive_sha256": "cd",
		"targets": "", "deps": "{}", "size": int64(len(archive)),
		"storage_key": archiveKey, "user_id": owner.ID,
	})
	if err != nil {
		t.Fatalf("create version row: %v", err)
	}
	if _, err := repo.CreateFrom[models.Download](sql.H{"version_id": row.ID, "date": "2026-10-17", "count": 3}); err != nil {
		t.Fatalf("create download row: %v", err)
	}
	return row
}

func setupPurgeTest(t *testing.T) (*gin.Engine, *models.Session, *models.Package, *models.Version, *models.Version) {
	t.Helper()

	if _, err := storage.Setup(storage.Config{Driver: storage.DriverLocal, Root: t.TempDir()}); err != nil {
		t.Fatalf("storage.Setup: %v", err)
	}

	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	owner, err := repo.CreateFrom[models.User](sql.H{
		"username": "bob", "email": "bob@example.com", "password_digest": "x",
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}

	victim, err := repo.CreateFrom[models.Package](sql.H{
		"owner_scope": "bob", "name": "victim",
		"description": "", "license": "", "homepage": "", "repository": "",
		"user_id": owner.ID, "downloads": 0, "visibility": models.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create victim: %v", err)
	}
	v1 := seedPublishedVersion(t, victim, owner, "0.1.0")
	v2 := seedPublishedVersion(t, victim, owner, "0.2.0")

	// A sibling package under the same owner must survive the purge.
	keeper, err := repo.CreateFrom[models.Package](sql.H{
		"owner_scope": "bob", "name": "keeper",
		"description": "", "license": "", "homepage": "", "repository": "",
		"user_id": owner.ID, "downloads": 0, "visibility": models.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create keeper: %v", err)
	}
	seedPublishedVersion(t, keeper, owner, "1.0.0")

	return r, admin, victim, v1, v2
}

func TestPurgeRemovesPackageFromDBAndStorage(t *testing.T) {
	r, admin, victim, v1, v2 := setupPurgeTest(t)

	resp := do(t, r, http.MethodPost, "/admin/packages/bob/victim/purge",
		url.Values{"confirm": {"I want to purge this package bob/victim"}}, admin)
	if resp.Code != http.StatusFound {
		t.Fatalf("purge: expected 302, got %d", resp.Code)
	}

	if row, _ := repo.FindOneBy[models.Package](sql.H{"owner_scope": "bob", "name": "victim"}); row != nil {
		t.Fatalf("package row must be gone")
	}
	if rows, _ := repo.FindBy[models.Version](sql.H{"package_id": victim.ID}); len(rows) != 0 {
		t.Fatalf("version rows must be gone")
	}
	for _, v := range []*models.Version{v1, v2} {
		if rows, _ := repo.FindBy[models.Download](sql.H{"version_id": v.ID}); len(rows) != 0 {
			t.Fatalf("download rows for %s must be gone", v.Version)
		}
	}

	store := storage.Current()
	ctx := context.Background()
	for _, key := range []string{
		v1.StorageKey,
		strings.TrimSuffix(v1.StorageKey, "/package.emoji") + "/files/package.emo",
		v2.StorageKey,
		strings.TrimSuffix(v2.StorageKey, "/package.emoji") + "/files/" + victim.Name + ".emo",
	} {
		if exists, _ := store.Exists(ctx, key); exists {
			t.Fatalf("storage key %s must be gone", key)
		}
	}

	keeper, _ := repo.FindOneBy[models.Package](sql.H{"owner_scope": "bob", "name": "keeper"})
	if keeper == nil {
		t.Fatalf("keeper package must survive")
	}
	if versions, _ := repo.FindBy[models.Version](sql.H{"package_id": keeper.ID}); len(versions) != 1 {
		t.Fatalf("keeper version must survive")
	}
	if exists, _ := store.Exists(ctx, "packages/bob/keeper/1.0.0/package.emoji"); !exists {
		t.Fatalf("keeper archive must survive")
	}
}

// The typed phrase is the only gate — a mismatch re-renders the list and
// deletes nothing.
func TestPurgeRejectsWrongPhrase(t *testing.T) {
	r, admin, victim, _, _ := setupPurgeTest(t)

	resp := do(t, r, http.MethodPost, "/admin/packages/bob/victim/purge",
		url.Values{"confirm": {"I want to purge this package bob/keeper"}}, admin)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong phrase: expected 422, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "nothing was deleted") {
		t.Fatalf("expected the mismatch error on the re-rendered list")
	}

	if rows, _ := repo.FindBy[models.Version](sql.H{"package_id": victim.ID}); len(rows) != 2 {
		t.Fatalf("versions must survive a rejected purge")
	}
	if exists, _ := storage.Current().Exists(context.Background(), v1key(t, victim)); !exists {
		t.Fatalf("archive must survive a rejected purge")
	}
}

func TestPurgeUnknownPackageIs404(t *testing.T) {
	r, admin, _, _, _ := setupPurgeTest(t)

	resp := do(t, r, http.MethodPost, "/admin/packages/bob/nope/purge",
		url.Values{"confirm": {"I want to purge this package bob/nope"}}, admin)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("unknown package: expected 404, got %d", resp.Code)
	}
}

// Regression: the dialog's confirmation input must submit under the name the
// server reads. Without name="confirm" the browser posts an empty field —
// the JS check still passes (it reads .value) and every purge 422s.
// The submit button starts disabled and carries a visible disabled style so
// an empty or mismatched phrase reads as inert.
func TestPurgeDialogInputSubmitsAsConfirm(t *testing.T) {
	r, admin, _, _, _ := setupPurgeTest(t)

	resp := do(t, r, http.MethodGet, "/admin", nil, admin)
	body := resp.Body.String()
	if !strings.Contains(body, `id="purge-confirm" name="confirm"`) {
		t.Fatalf("the dialog input must carry name=\"confirm\" so the typed phrase is submitted")
	}
	if !strings.Contains(body, `id="purge-submit" disabled`) {
		t.Fatalf("the submit button must start disabled")
	}
	if !strings.Contains(body, "#purge-submit:disabled") {
		t.Fatalf("the submit button must ship a visible disabled style")
	}
}

func v1key(t *testing.T, pkg *models.Package) string {
	t.Helper()
	row, err := repo.FindOneBy[models.Version](sql.H{"package_id": pkg.ID, "version": "0.1.0"})
	if err != nil || row == nil {
		t.Fatalf("lookup v1: %v", err)
	}
	return row.StorageKey
}
