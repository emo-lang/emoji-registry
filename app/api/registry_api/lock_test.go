package registry_api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/lock"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// seedAccount creates a user with a push,yank token straight in the repo —
// the signup endpoint's per-IP rate limit is a process-wide budget the
// package's other tests already exhaust.
func seedAccount(t *testing.T, username string) string {
	t.Helper()

	user, err := repo.CreateFrom[models.User](sql.H{
		"username": username, "email": username + "@example.com", "password_digest": "unused",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	plain := middlewares.TokenPrefix + strings.Repeat("cd", 24)
	if _, err := repo.CreateFrom[models.APIToken](sql.H{
		"user_id": user.ID, "name": "cli",
		"token_hash": middlewares.TokenHash(plain), "scopes": "push,yank",
	}); err != nil {
		t.Fatalf("create token: %v", err)
	}

	return plain
}

func setLocked(t *testing.T, username string, locked bool) {
	t.Helper()

	user, err := repo.FindOneBy[models.User](sql.H{"username": username})
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	if err := repo.UpdateByID[models.User](user.ID, sql.H{"locked": locked}); err != nil {
		t.Fatalf("set locked: %v", err)
	}
}

func errorMessage(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSON(t, resp, &body)
	return body.Error.Message
}

func TestLockedUserScope(t *testing.T) {
	r := setupTest(t)

	token := seedAccount(t, "alice")
	if resp := publish(t, r, token, "alice/tools", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}

	setLocked(t, "alice", true)

	// B.4 keeps describing the package: a locked scope stays listed and its
	// page keeps working.
	resp := do(t, r, http.MethodGet, "/api/v1/packages/alice/tools", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("package metadata under lock: expected 200, got %d %s", resp.Code, resp.Body.String())
	}

	// Resolution inputs (B.1, B.2), the archive download (B.3) and protocol A
	// all refuse the scope with the shared error.
	for _, target := range []string{
		"/api/v1/packages/alice/tools/versions",
		"/api/v1/dependencies?packages=alice/tools",
		"/downloads/alice-tools-1.0.0.emoji",
		"/alice/tools/versions",
		"/alice/tools/1.0.0/package.emo",
	} {
		resp := do(t, r, http.MethodGet, target, nil, nil)
		if resp.Code != http.StatusForbidden || errorCode(t, resp) != lock.Code {
			t.Fatalf("locked %s: expected 403 %s, got %d %s", target, lock.Code, resp.Code, resp.Body.String())
		}
		if msg := errorMessage(t, resp); msg != lock.Message {
			t.Fatalf("locked %s: expected message %q, got %q", target, lock.Message, msg)
		}
	}

	// Writes under the scope are refused the same way.
	if resp := publish(t, r, token, "alice/tools", "1.1.0"); resp.Code != http.StatusForbidden || errorCode(t, resp) != lock.Code {
		t.Fatalf("publish under lock: expected 403 %s, got %d %s", lock.Code, resp.Code, resp.Body.String())
	}
	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/1.0.0", nil, bearer(token))
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != lock.Code {
		t.Fatalf("yank under lock: expected 403 %s, got %d %s", lock.Code, resp.Code, resp.Body.String())
	}
	resp = do(t, r, http.MethodPatch, "/api/v1/packages/alice/tools",
		[]byte(`{"description":"nope"}`),
		mergeHeaders(bearer(token), map[string]string{"Content-Type": "application/json"}))
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != lock.Code {
		t.Fatalf("patch under lock: expected 403 %s, got %d %s", lock.Code, resp.Code, resp.Body.String())
	}

	// Unlocking is immediately effective.
	setLocked(t, "alice", false)
	resp = do(t, r, http.MethodGet, "/downloads/alice-tools-1.0.0.emoji", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("download after unlock: expected 200, got %d", resp.Code)
	}
	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/tools/versions", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("versions after unlock: expected 200, got %d", resp.Code)
	}
}

func TestLockedOrganizationScope(t *testing.T) {
	r := setupTest(t)

	token := seedAccount(t, "alice")
	user, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	org, err := orgs.Create(user, "acme", "")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	if resp := publish(t, r, token, "acme/tools", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}

	if err := repo.UpdateByID[models.Organization](org.ID, sql.H{"locked": true}); err != nil {
		t.Fatalf("lock org: %v", err)
	}

	for _, target := range []string{
		"/api/v1/packages/acme/tools/versions",
		"/api/v1/dependencies?packages=acme/tools",
		"/downloads/acme-tools-1.0.0.emoji",
	} {
		resp := do(t, r, http.MethodGet, target, nil, nil)
		if resp.Code != http.StatusForbidden || errorCode(t, resp) != lock.Code {
			t.Fatalf("locked org %s: expected 403 %s, got %d %s", target, lock.Code, resp.Code, resp.Body.String())
		}
	}
}

func TestLockedPrivatePackageStaysInvisible(t *testing.T) {
	r := setupTest(t)

	token := seedAccount(t, "alice")
	if resp := publish(t, r, token, "alice/secret", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}
	privatize(t, r, token, "alice/secret")
	setLocked(t, "alice", true)

	// The lock must not turn a private package's 404 into a 403: outsiders
	// still learn nothing about it.
	for _, target := range []string{
		"/api/v1/packages/alice/secret/versions",
		"/downloads/alice-secret-1.0.0.emoji",
		"/alice/secret/versions",
	} {
		resp := do(t, r, http.MethodGet, target, nil, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("private under lock %s: expected 404, got %d %s", target, resp.Code, resp.Body.String())
		}
	}

	resp := do(t, r, http.MethodGet, "/api/v1/dependencies?packages=alice/secret", nil, nil)
	if resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "alice/secret") {
		t.Fatalf("private under lock dependencies: %d %s", resp.Code, resp.Body.String())
	}
}
