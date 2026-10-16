package admin_api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

// seedPkg inserts a package owned by owner with created_at pinned to
// base+age, so list ordering is deterministic regardless of insert order.
func seedPkg(t *testing.T, owner *models.User, base time.Time, age time.Duration, name, visibility string) *models.Package {
	t.Helper()

	pkg, err := repo.CreateFrom[models.Package](sql.H{
		"owner_scope": owner.Username, "name": name,
		"description": "", "license": "", "homepage": "", "repository": "",
		"user_id": owner.ID, "downloads": 0, "visibility": visibility,
	})
	if err != nil {
		t.Fatalf("create package %s: %v", name, err)
	}
	if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{"created_at": base.Add(age)}); err != nil {
		t.Fatalf("backdate package %s: %v", name, err)
	}
	return pkg
}

func seedVersion(t *testing.T, pkg *models.Package, owner *models.User, version string) {
	t.Helper()

	if _, err := repo.CreateFrom[models.Version](sql.H{
		"package_id": pkg.ID, "version": version,
		"checksum": "ab", "archive_sha256": "cd",
		"targets": "", "deps": "{}", "size": 10, "storage_key": "k",
		"user_id": owner.ID,
	}); err != nil {
		t.Fatalf("create version %s: %v", version, err)
	}
}

func TestPackagesListNewestFirst(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	owner, err := repo.CreateFrom[models.User](sql.H{
		"username": "bob", "email": "bob@example.com", "password_digest": "x",
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}

	base := time.Now().Add(-2 * time.Hour)
	older := seedPkg(t, owner, base, 0, "older", models.VisibilityPublic)
	newest := seedPkg(t, owner, base, time.Hour, "newest", models.VisibilityPrivate)
	seedPkg(t, owner, base, 30*time.Minute, "middle", models.VisibilityPublic)

	seedVersion(t, older, owner, "0.1.0")
	seedVersion(t, newest, owner, "0.1.0")
	seedVersion(t, newest, owner, "0.2.0")

	resp := do(t, r, http.MethodGet, "/admin", nil, admin)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()

	// Newest first: newest, then middle, then older.
	last := -1
	for _, name := range []string{"bob/newest", "bob/middle", "bob/older"} {
		at := strings.Index(body, name)
		if at < 0 || at < last {
			t.Fatalf("expected %s after the previous row in the list", name)
		}
		last = at
	}

	// Private packages are listed with their visibility.
	if !strings.Contains(body, `class="chip private"`) {
		t.Fatalf("expected the private visibility chip")
	}
	// bob/newest carries two versions.
	if !strings.Contains(body, ">2</td>") {
		t.Fatalf("expected a version count of 2")
	}
}

// Anonymous and non-admin visitors must not see the package list.
func TestPackagesListHiddenFromNonAdmins(t *testing.T) {
	r := setupTest(t)

	if resp := do(t, r, http.MethodGet, "/admin", nil, nil); resp.Code != http.StatusNotFound {
		t.Fatalf("anonymous: expected 404, got %d", resp.Code)
	}

	member := makeUser(t, "bob", false)
	if resp := do(t, r, http.MethodGet, "/admin", nil, member); resp.Code != http.StatusNotFound {
		t.Fatalf("member: expected 404, got %d", resp.Code)
	}
}
