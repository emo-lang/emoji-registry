package registry_api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func privatize(t *testing.T, r *gin.Engine, token, fullName string) {
	t.Helper()

	owner, name, _ := strings.Cut(fullName, "/")
	resp := do(t, r, http.MethodPatch, "/api/v1/packages/"+owner+"/"+name,
		[]byte(`{"visibility":"private"}`),
		mergeHeaders(bearer(token), map[string]string{"Content-Type": "application/json"}))
	if resp.Code != http.StatusOK {
		t.Fatalf("privatize: %d %s", resp.Code, resp.Body.String())
	}
}

func TestPrivatePackageAccess(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	signup(t, r, "bob")

	aliceFull := createToken(t, r, "alice", "push", "yank", "read")
	aliceReadOnly := createToken(t, r, "alice", "read")
	alicePushOnly := createToken(t, r, "alice", "push")
	bobRead := createToken(t, r, "bob", "read")

	if resp := publish(t, r, aliceFull, "alice/secret", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}

	readEndpoints := []string{
		"/api/v1/packages/alice/secret",
		"/api/v1/packages/alice/secret/versions",
		"/downloads/alice--secret--1.0.0.emoji",
		"/alice/secret/versions",
		"/alice/secret/1.0.0/package.emo",
	}

	// Public by default: anonymous reads work.
	for _, path := range readEndpoints {
		if resp := do(t, r, http.MethodGet, path, nil, nil); resp.Code != http.StatusOK {
			t.Fatalf("public %s: expected 200, got %d", path, resp.Code)
		}
	}

	privatize(t, r, aliceFull, "alice/secret")

	// Anonymous and non-members get the same 404 as unknown packages.
	for _, path := range readEndpoints {
		resp := do(t, r, http.MethodGet, path, nil, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("anonymous %s: expected 404, got %d", path, resp.Code)
		}
		if strings.HasPrefix(path, "/api/") && errorCode(t, resp) != "package_not_found" {
			t.Fatalf("anonymous %s: expected package_not_found, got %s", path, resp.Body.String())
		}

		resp = do(t, r, http.MethodGet, path, nil, bearer(bobRead))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("bob %s: expected 404, got %d", path, resp.Code)
		}
	}

	// A token without the read scope cannot read even its owner's package.
	resp := do(t, r, http.MethodGet, "/api/v1/packages/alice/secret", nil, bearer(alicePushOnly))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("push-only token: expected 404, got %d", resp.Code)
	}

	// The owner's read-scoped token gets in everywhere.
	for _, path := range readEndpoints {
		if resp := do(t, r, http.MethodGet, path, nil, bearer(aliceReadOnly)); resp.Code != http.StatusOK {
			t.Fatalf("owner read token %s: expected 200, got %d", path, resp.Code)
		}
	}

	// Dependencies query: the private package is absent for outsiders, present
	// for the owner.
	resp = do(t, r, http.MethodGet, "/api/v1/dependencies?packages=alice/secret", nil, nil)
	if strings.Contains(resp.Body.String(), "alice/secret") {
		t.Fatalf("dependencies leaked the private package: %s", resp.Body.String())
	}
	resp = do(t, r, http.MethodGet, "/api/v1/dependencies?packages=alice/secret", nil, bearer(aliceReadOnly))
	if !strings.Contains(resp.Body.String(), "alice/secret") {
		t.Fatalf("dependencies should list the package for the owner: %s", resp.Body.String())
	}

	// Yanked + private: still listed with the flag for the owner, still
	// downloadable, invisible to everyone else.
	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/secret/versions/1.0.0", nil, bearer(aliceFull))
	if resp.Code != http.StatusOK {
		t.Fatalf("yank: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/secret/versions", nil, bearer(aliceReadOnly))
	if !strings.Contains(resp.Body.String(), `"yanked":true`) {
		t.Fatalf("owner should see the yanked version: %s", resp.Body.String())
	}
	resp = do(t, r, http.MethodGet, "/downloads/alice--secret--1.0.0.emoji", nil, bearer(aliceReadOnly))
	if resp.Code != http.StatusOK || resp.Header().Get("X-Emo-Yanked") != "true" {
		t.Fatalf("yanked private download: %d %#v", resp.Code, resp.Header())
	}
	resp = do(t, r, http.MethodGet, "/alice/secret/versions", nil, bearer(aliceReadOnly))
	if resp.Body.String() != "[]" {
		t.Fatalf("protocol A must exclude yanked: %s", resp.Body.String())
	}

	// Flipping back to public takes effect immediately.
	resp = do(t, r, http.MethodPatch, "/api/v1/packages/alice/secret",
		[]byte(`{"visibility":"public"}`),
		mergeHeaders(bearer(aliceFull), map[string]string{"Content-Type": "application/json"}))
	if resp.Code != http.StatusOK {
		t.Fatalf("make public: %d %s", resp.Code, resp.Body.String())
	}
	if resp := do(t, r, http.MethodGet, "/api/v1/packages/alice/secret", nil, nil); resp.Code != http.StatusOK {
		t.Fatalf("public again: expected 200, got %d", resp.Code)
	}
}

func TestOrgPrivatePackageVisibleToMembers(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	signup(t, r, "bob")

	aliceToken := createToken(t, r, "alice", "push", "read")

	resp := do(t, r, http.MethodPost, "/api/v1/orgs", []byte(`{"name":"acme"}`), basicAuth2("alice"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("create org: %d %s", resp.Code, resp.Body.String())
	}

	if resp := publish(t, r, aliceToken, "acme/secret", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}
	privatize(t, r, aliceToken, "acme/secret")

	// Bob is not a member yet: 404 even with a read token.
	bobRead := createToken(t, r, "bob", "read")
	if resp := do(t, r, http.MethodGet, "/api/v1/packages/acme/secret", nil, bearer(bobRead)); resp.Code != http.StatusNotFound {
		t.Fatalf("non-member: expected 404, got %d", resp.Code)
	}

	// Add bob; his read token works immediately.
	resp = do(t, r, http.MethodPost, "/api/v1/orgs/acme/members", []byte(`{"username":"bob"}`), basicAuth2("alice"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("add bob: %d %s", resp.Code, resp.Body.String())
	}
	if resp := do(t, r, http.MethodGet, "/api/v1/packages/acme/secret", nil, bearer(bobRead)); resp.Code != http.StatusOK {
		t.Fatalf("member read token: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
}

func basicAuth2(username string) map[string]string {
	return map[string]string{
		"Authorization": "Basic " + basicAuth(username+"@example.com", "super-secret"),
		"Content-Type":  "application/json",
	}
}
