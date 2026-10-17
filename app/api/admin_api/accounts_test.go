package admin_api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

func TestAccountsInvisibleWithoutAdmin(t *testing.T) {
	r := setupTest(t)

	if resp := do(t, r, http.MethodGet, "/admin/accounts", nil, nil); resp.Code != http.StatusNotFound {
		t.Fatalf("anonymous: expected 404, got %d", resp.Code)
	}

	member := makeUser(t, "bob", false)
	if resp := do(t, r, http.MethodGet, "/admin/accounts", nil, member); resp.Code != http.StatusNotFound {
		t.Fatalf("member: expected 404, got %d", resp.Code)
	}
}

func TestAccountsListAndLockToggle(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)
	makeUser(t, "bob", false)

	bob, err := repo.FindOneBy[models.User](sql.H{"username": "bob"})
	if err != nil || bob == nil {
		t.Fatalf("find bob: %v", err)
	}

	// The list renders the account with a lock form.
	resp := do(t, r, http.MethodGet, "/admin/accounts", nil, admin)
	if resp.Code != http.StatusOK {
		t.Fatalf("accounts: expected 200, got %d", resp.Code)
	}
	lockPath := "/admin/accounts/users/" + strconv.FormatInt(int64(bob.ID), 10) + "/lock"
	if body := resp.Body.String(); !strings.Contains(body, ">bob<") || !strings.Contains(body, lockPath) {
		t.Fatalf("expected bob's row with %s, got %q", lockPath, body)
	}

	// Lock: the flag flips and the page shows the locked state.
	if resp := do(t, r, http.MethodPost, lockPath, nil, admin); resp.Code != http.StatusFound {
		t.Fatalf("lock: expected 302, got %d", resp.Code)
	}
	bob, err = repo.FindOneBy[models.User](sql.H{"username": "bob"})
	if err != nil || bob == nil {
		t.Fatalf("reload bob: %v", err)
	}
	if !bob.Locked {
		t.Fatalf("expected bob to be locked")
	}

	resp = do(t, r, http.MethodGet, "/admin/accounts", nil, admin)
	if body := resp.Body.String(); !strings.Contains(body, "已锁定") {
		t.Fatalf("expected the locked chip on the page, got %q", body)
	}

	// Unlock restores.
	unlockPath := "/admin/accounts/users/" + strconv.FormatInt(int64(bob.ID), 10) + "/unlock"
	if resp := do(t, r, http.MethodPost, unlockPath, nil, admin); resp.Code != http.StatusFound {
		t.Fatalf("unlock: expected 302, got %d", resp.Code)
	}
	bob, err = repo.FindOneBy[models.User](sql.H{"username": "bob"})
	if err != nil || bob == nil {
		t.Fatalf("reload bob: %v", err)
	}
	if bob.Locked {
		t.Fatalf("expected bob to be unlocked")
	}
}

func TestLockOrganization(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	bob, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || bob == nil {
		t.Fatalf("find alice: %v", err)
	}
	org, err := repo.CreateFrom[models.Organization](sql.H{
		"name": "acme", "display_name": "", "user_id": bob.ID,
	})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	path := "/admin/accounts/orgs/" + strconv.FormatInt(int64(org.ID), 10) + "/lock"
	if resp := do(t, r, http.MethodPost, path, nil, admin); resp.Code != http.StatusFound {
		t.Fatalf("lock org: expected 302, got %d", resp.Code)
	}

	stored, err := repo.FindByID[models.Organization](org.ID)
	if err != nil || stored == nil {
		t.Fatalf("reload org: %v", err)
	}
	if !stored.Locked {
		t.Fatalf("expected the organization to be locked")
	}
}

func TestLockUnknownKindOrID(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	for _, path := range []string{
		"/admin/accounts/widgets/1/lock",
		"/admin/accounts/users/not-a-number/lock",
	} {
		if resp := do(t, r, http.MethodPost, path, nil, admin); resp.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", path, resp.Code)
		}
	}
}
