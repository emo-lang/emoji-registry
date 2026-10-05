package registry_api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/api/accounts_api"
	"github.com/emo-lang/emoji-registry/app/api/downloads_api"
	"github.com/emo-lang/emoji-registry/app/api/files_api"
	"github.com/emo-lang/emoji-registry/app/api/orgs_api"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupTest(t *testing.T) *gin.Engine {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))

	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}
	if _, err := storage.Setup(storage.Config{Driver: storage.DriverLocal, Root: t.TempDir()}); err != nil {
		t.Fatalf("setup storage: %v", err)
	}

	gin.SetMode(gin.TestMode)

	r := gin.New()
	v1 := r.Group("/api/v1")
	accounts_api.Routes(v1)
	Routes(v1)
	orgs_api.APIRoutes(v1)
	downloads_api.Routes(r)
	files_api.Routes(r)

	return r
}

func do(t *testing.T, r *gin.Engine, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func decodeJSON(t *testing.T, resp *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), into); err != nil {
		t.Fatalf("decode %s: %v", resp.Body.String(), err)
	}
}

func errorCode(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, resp, &body)
	return body.Error.Code
}

func signup(t *testing.T, r *gin.Engine, username string) {
	t.Helper()

	body := fmt.Sprintf(`{"username":%q,"email":%q,"password":"super-secret"}`, username, username+"@example.com")
	resp := do(t, r, http.MethodPost, "/api/v1/signup", []byte(body), map[string]string{"Content-Type": "application/json"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("signup %s: expected 201, got %d: %s", username, resp.Code, resp.Body.String())
	}
}

func createToken(t *testing.T, r *gin.Engine, username string, scopes ...string) string {
	t.Helper()

	scopeJSON, _ := json.Marshal(scopes)
	body := fmt.Sprintf(`{"name":"cli","scopes":%s}`, scopeJSON)

	resp := do(t, r, http.MethodPost, "/api/v1/tokens", []byte(body), map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Basic " + basicAuth(username+"@example.com", "super-secret"),
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create token: expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var out struct {
		Token string `json:"token"`
	}
	decodeJSON(t, resp, &out)

	if !strings.HasPrefix(out.Token, "emo_") || len(out.Token) != 4+48 {
		t.Fatalf("unexpected token format: %q", out.Token)
	}

	return out.Token
}

func basicAuth(user, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func buildPackage(t *testing.T, fullName, version string, sources map[string]string) []byte {
	t.Helper()

	manifest := fmt.Sprintf("package {\n  name = %q\n  version = %q\n  targets = [\"native\"]\n}\n", fullName, version)

	files := map[string][]byte{"package.emo": []byte(manifest)}
	for name, content := range sources {
		files[name] = []byte(content)
	}

	archive, err := emoji.BuildArchive(files)
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}
	return archive
}

func publish(t *testing.T, r *gin.Engine, token, fullName, version string) *httptest.ResponseRecorder {
	t.Helper()
	archive := buildPackage(t, fullName, version, map[string]string{"main.emo": "let main = 1 // v" + version})
	return do(t, r, http.MethodPost, "/api/v1/packages", archive, bearer(token))
}

func TestFullRegistryFlow(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	token := createToken(t, r, "alice", "push", "yank")

	// Publish two versions, out of order to exercise semver sorting.
	resp := publish(t, r, token, "alice/tools", "0.2.0")
	if resp.Code != http.StatusCreated {
		t.Fatalf("publish 0.2.0: expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = publish(t, r, token, "alice/tools", "0.1.0")
	if resp.Code != http.StatusCreated {
		t.Fatalf("publish 0.1.0: expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var published struct {
		Version  string `json:"version"`
		Checksum string `json:"checksum"`
		Yanked   bool   `json:"yanked"`
	}
	decodeJSON(t, resp, &published)
	if published.Version != "0.1.0" || published.Yanked || len(published.Checksum) != 64 {
		t.Fatalf("unexpected publish response: %+v", published)
	}

	// Re-publishing an existing version is a conflict.
	resp = publish(t, r, token, "alice/tools", "0.1.0")
	if resp.Code != http.StatusConflict || errorCode(t, resp) != "version_exists" {
		t.Fatalf("re-publish: expected 409 version_exists, got %d: %s", resp.Code, resp.Body.String())
	}

	// B.1 version list, ascending semver.
	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/tools/versions", nil, nil)
	var versionList struct {
		Package  string `json:"package"`
		Versions []struct {
			Version string `json:"version"`
			Yanked  bool   `json:"yanked"`
		} `json:"versions"`
	}
	decodeJSON(t, resp, &versionList)
	if versionList.Package != "alice/tools" || len(versionList.Versions) != 2 ||
		versionList.Versions[0].Version != "0.1.0" || versionList.Versions[1].Version != "0.2.0" {
		t.Fatalf("unexpected version list: %s", resp.Body.String())
	}

	// B.4 package metadata.
	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/tools", nil, nil)
	var detail struct {
		Name          string   `json:"name"`
		Owners        []string `json:"owners"`
		LatestVersion string   `json:"latest_version"`
		Downloads     int64    `json:"downloads"`
	}
	decodeJSON(t, resp, &detail)
	if detail.Name != "alice/tools" || detail.LatestVersion != "0.2.0" ||
		len(detail.Owners) != 1 || detail.Owners[0] != "alice" {
		t.Fatalf("unexpected package detail: %s", resp.Body.String())
	}

	// B.2 dependencies; unknown packages are absent.
	resp = do(t, r, http.MethodGet, "/api/v1/dependencies?packages=alice/tools,ghost/pkg", nil, nil)
	var deps map[string][]struct {
		Version string            `json:"version"`
		Deps    map[string]string `json:"deps"`
		Yanked  bool              `json:"yanked"`
	}
	decodeJSON(t, resp, &deps)
	if _, ok := deps["ghost/pkg"]; ok {
		t.Fatalf("unknown package must be absent: %s", resp.Body.String())
	}
	if len(deps["alice/tools"]) != 2 {
		t.Fatalf("unexpected dependencies: %s", resp.Body.String())
	}

	// B.3 download, with integrity headers.
	resp = do(t, r, http.MethodGet, "/downloads/alice--tools--0.1.0.emoji", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("unexpected content type: %q", ct)
	}
	if !strings.HasPrefix(resp.Header().Get("Digest"), "sha-256=") || resp.Header().Get("ETag") == "" {
		t.Fatalf("missing integrity headers: %#v", resp.Header())
	}
	if _, err := gzip.NewReader(bytes.NewReader(resp.Body.Bytes())); err != nil {
		t.Fatalf("download body is not gzip: %v", err)
	}

	// Protocol A: version list, manifest and source files.
	resp = do(t, r, http.MethodGet, "/alice/tools/versions", nil, nil)
	var protocolAVersions []string
	decodeJSON(t, resp, &protocolAVersions)
	if len(protocolAVersions) != 2 || protocolAVersions[0] != "0.1.0" || protocolAVersions[1] != "0.2.0" {
		t.Fatalf("unexpected protocol A versions: %s", resp.Body.String())
	}

	resp = do(t, r, http.MethodGet, "/alice/tools/0.1.0/package.emo", nil, nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `name = "alice/tools"`) {
		t.Fatalf("protocol A manifest: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, r, http.MethodGet, "/alice/tools/0.1.0/main.emo", nil, nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "let main = 1") {
		t.Fatalf("protocol A source: %d %s", resp.Code, resp.Body.String())
	}

	// Non-.emo paths and traversal attempts are not served.
	for _, path := range []string{
		"/alice/tools/0.1.0/EMO-METADATA.json",
		"/alice/tools/0.1.0/../package.emo",
	} {
		resp = do(t, r, http.MethodGet, path, nil, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("protocol A must not serve %s: got %d", path, resp.Code)
		}
	}

	// Yank 0.1.0; yanking twice is idempotent.
	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/0.1.0", nil, bearer(token))
	if resp.Code != http.StatusOK {
		t.Fatalf("yank: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var yanked struct {
		Yanked bool `json:"yanked"`
	}
	decodeJSON(t, resp, &yanked)
	if !yanked.Yanked {
		t.Fatalf("expected yanked version object: %s", resp.Body.String())
	}

	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/0.1.0", nil, bearer(token))
	if resp.Code != http.StatusOK {
		t.Fatalf("re-yank: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	// The yanked version stays in the B.1 list with the flag set, is gone
	// from protocol A, yet remains downloadable with a warning header.
	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/tools/versions", nil, nil)
	decodeJSON(t, resp, &versionList)
	if len(versionList.Versions) != 2 || !versionList.Versions[0].Yanked {
		t.Fatalf("yanked version must stay in the list: %s", resp.Body.String())
	}

	resp = do(t, r, http.MethodGet, "/alice/tools/versions", nil, nil)
	decodeJSON(t, resp, &protocolAVersions)
	if len(protocolAVersions) != 1 || protocolAVersions[0] != "0.2.0" {
		t.Fatalf("protocol A must exclude yanked versions: %s", resp.Body.String())
	}

	resp = do(t, r, http.MethodGet, "/alice/tools/0.1.0/package.emo", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("protocol A must 404 yanked versions, got %d", resp.Code)
	}

	resp = do(t, r, http.MethodGet, "/downloads/alice--tools--0.1.0.emoji", nil, nil)
	if resp.Code != http.StatusOK || resp.Header().Get("X-Emo-Yanked") != "true" {
		t.Fatalf("yanked download: %d %#v", resp.Code, resp.Header())
	}

	// latest_version skips yanked versions; downloads were counted.
	resp = do(t, r, http.MethodGet, "/api/v1/packages/alice/tools", nil, nil)
	decodeJSON(t, resp, &detail)
	if detail.Downloads != 2 {
		t.Fatalf("expected 2 downloads, got %d", detail.Downloads)
	}

	// Owner-only metadata editing.
	resp = do(t, r, http.MethodPatch, "/api/v1/packages/alice/tools",
		[]byte(`{"description":"JSON utilities","license":"MIT"}`),
		mergeHeaders(bearer(token), map[string]string{"Content-Type": "application/json"}))
	if resp.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var patched struct {
		Description string `json:"description"`
		License     string `json:"license"`
	}
	decodeJSON(t, resp, &patched)
	if patched.Description != "JSON utilities" || patched.License != "MIT" {
		t.Fatalf("unexpected patch result: %s", resp.Body.String())
	}
}

func TestAuthorizationRules(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	signup(t, r, "bob")

	aliceToken := createToken(t, r, "alice", "push", "yank")
	pushOnly := createToken(t, r, "alice", "push")
	bobToken := createToken(t, r, "bob", "push", "yank")

	if resp := publish(t, r, aliceToken, "alice/tools", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}

	// No token at all.
	resp := do(t, r, http.MethodPost, "/api/v1/packages", buildPackage(t, "alice/x", "1.0.0", nil), nil)
	if resp.Code != http.StatusUnauthorized || errorCode(t, resp) != "unauthorized" {
		t.Fatalf("expected 401 unauthorized, got %d: %s", resp.Code, resp.Body.String())
	}

	// A push-scoped token cannot yank.
	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/1.0.0", nil, bearer(pushOnly))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing yank scope, got %d: %s", resp.Code, resp.Body.String())
	}

	// Bob can neither publish under alice's scope nor yank alice's package.
	resp = publish(t, r, bobToken, "alice/tools", "2.0.0")
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != "forbidden" {
		t.Fatalf("expected 403 forbidden, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/1.0.0", nil, bearer(bobToken))
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != "forbidden" {
		t.Fatalf("expected 403 forbidden, got %d: %s", resp.Code, resp.Body.String())
	}

	// A manifest whose owner does not match the token account is rejected.
	resp = publish(t, r, bobToken, "carol/tools", "1.0.0")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign scope, got %d: %s", resp.Code, resp.Body.String())
	}

	// Bob can publish his own package, but not over alice's name.
	resp = publish(t, r, bobToken, "bob/tools", "1.0.0")
	if resp.Code != http.StatusCreated {
		t.Fatalf("bob publish: %d %s", resp.Code, resp.Body.String())
	}

	// Broken archives surface as invalid_manifest.
	resp = do(t, r, http.MethodPost, "/api/v1/packages", []byte("not a gzip"), bearer(aliceToken))
	if resp.Code != http.StatusUnprocessableEntity || errorCode(t, resp) != "invalid_manifest" {
		t.Fatalf("expected 422 invalid_manifest, got %d: %s", resp.Code, resp.Body.String())
	}

	// Unknown packages and versions answer with the shared error shape.
	resp = do(t, r, http.MethodGet, "/api/v1/packages/ghost/pkg", nil, nil)
	if resp.Code != http.StatusNotFound || errorCode(t, resp) != "package_not_found" {
		t.Fatalf("expected 404 package_not_found, got %d", resp.Code)
	}
	resp = do(t, r, http.MethodDelete, "/api/v1/packages/alice/tools/versions/9.9.9", nil, bearer(aliceToken))
	if resp.Code != http.StatusNotFound || errorCode(t, resp) != "version_not_found" {
		t.Fatalf("expected 404 version_not_found, got %d", resp.Code)
	}
}

func TestReservedNames(t *testing.T) {
	r := setupTest(t)

	// Reserved stdlib names cannot be registered as accounts...
	body := `{"username":"net","email":"net@example.com","password":"super-secret"}`
	resp := do(t, r, http.MethodPost, "/api/v1/signup", []byte(body), map[string]string{"Content-Type": "application/json"})
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != "name_reserved" {
		t.Fatalf("expected 403 name_reserved, got %d: %s", resp.Code, resp.Body.String())
	}
}

func mergeHeaders(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
