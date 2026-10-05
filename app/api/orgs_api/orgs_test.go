package orgs_api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/api/accounts_api"
	"github.com/emo-lang/emoji-registry/app/api/registry_api"
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
	registry_api.Routes(v1)
	APIRoutes(v1)

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

func basic(username string) map[string]string {
	cred := base64.StdEncoding.EncodeToString([]byte(username + "@example.com:super-secret"))
	return map[string]string{"Authorization": "Basic " + cred, "Content-Type": "application/json"}
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func errorCode(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", resp.Body.String(), err)
	}
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
	resp := do(t, r, http.MethodPost, "/api/v1/tokens", []byte(fmt.Sprintf(`{"name":"cli","scopes":%s}`, scopeJSON)), basic(username))
	if resp.Code != http.StatusCreated {
		t.Fatalf("create token for %s: %d %s", username, resp.Code, resp.Body.String())
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	return out.Token
}

func publish(t *testing.T, r *gin.Engine, token, fullName, version string) *httptest.ResponseRecorder {
	t.Helper()

	manifest := fmt.Sprintf("package {\n  name = %q\n  version = %q\n}\n", fullName, version)
	archive, err := emoji.BuildArchive(map[string][]byte{
		"package.emo": []byte(manifest),
		"main.emo":    []byte("let main = 1"),
	})
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}

	return do(t, r, http.MethodPost, "/api/v1/packages", archive, bearer(token))
}

func TestOrgPublishingFlow(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	signup(t, r, "bob")
	signup(t, r, "carol")

	// Alice creates the acme organization and adds bob as a member.
	resp := do(t, r, http.MethodPost, "/api/v1/orgs", []byte(`{"name":"acme","display_name":"Acme Corp"}`), basic("alice"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("create org: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, r, http.MethodPost, "/api/v1/orgs/acme/members", []byte(`{"username":"bob"}`), basic("alice"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("add member: %d %s", resp.Code, resp.Body.String())
	}

	// Org info lists both members with roles.
	resp = do(t, r, http.MethodGet, "/api/v1/orgs/acme", nil, nil)
	var orgInfo struct {
		Name    string `json:"name"`
		Members []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &orgInfo); err != nil {
		t.Fatalf("decode org: %v", err)
	}
	if orgInfo.Name != "acme" || len(orgInfo.Members) != 2 ||
		orgInfo.Members[0].Username != "alice" || orgInfo.Members[0].Role != "owner" ||
		orgInfo.Members[1].Username != "bob" || orgInfo.Members[1].Role != "member" {
		t.Fatalf("unexpected org info: %s", resp.Body.String())
	}

	// Bob (a member) publishes under the org scope; carol (not a member) cannot.
	bobToken := createToken(t, r, "bob", "push", "yank")
	carolToken := createToken(t, r, "carol", "push", "yank")

	if resp := publish(t, r, bobToken, "acme/widgets", "0.1.0"); resp.Code != http.StatusCreated {
		t.Fatalf("bob publish: %d %s", resp.Code, resp.Body.String())
	}

	resp = publish(t, r, carolToken, "acme/evil", "0.1.0")
	if resp.Code != http.StatusForbidden || errorCode(t, resp) != "forbidden" {
		t.Fatalf("carol publish: expected 403 forbidden, got %d: %s", resp.Code, resp.Body.String())
	}

	// Members can also yank and edit metadata under the org scope.
	resp = do(t, r, http.MethodPatch, "/api/v1/packages/acme/widgets",
		[]byte(`{"description":"Org package"}`),
		map[string]string{"Authorization": "Bearer " + bobToken, "Content-Type": "application/json"})
	if resp.Code != http.StatusOK {
		t.Fatalf("bob patch: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, r, http.MethodDelete, "/api/v1/packages/acme/widgets/versions/0.1.0", nil, bearer(bobToken))
	if resp.Code != http.StatusOK {
		t.Fatalf("bob yank: %d %s", resp.Code, resp.Body.String())
	}

	// Non-owners cannot manage members.
	resp = do(t, r, http.MethodPost, "/api/v1/orgs/acme/members", []byte(`{"username":"carol"}`), basic("bob"))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("bob add member: expected 403, got %d: %s", resp.Code, resp.Body.String())
	}

	// Alice removes bob; he loses publish rights immediately.
	resp = do(t, r, http.MethodDelete, "/api/v1/orgs/acme/members/bob", nil, basic("alice"))
	if resp.Code != http.StatusOK {
		t.Fatalf("remove bob: %d %s", resp.Code, resp.Body.String())
	}
	resp = publish(t, r, bobToken, "acme/widgets", "0.2.0")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("bob publish after removal: expected 403, got %d: %s", resp.Code, resp.Body.String())
	}

	// The last owner cannot be removed.
	resp = do(t, r, http.MethodDelete, "/api/v1/orgs/acme/members/alice", nil, basic("alice"))
	if resp.Code != http.StatusConflict || errorCode(t, resp) != "last_owner" {
		t.Fatalf("remove last owner: expected 409 last_owner, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestOrgNameCollisions(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")

	// An organization cannot take a username...
	resp := do(t, r, http.MethodPost, "/api/v1/orgs", []byte(`{"name":"alice"}`), basic("alice"))
	if resp.Code != http.StatusConflict || errorCode(t, resp) != "name_taken" {
		t.Fatalf("org named alice: expected 409 name_taken, got %d: %s", resp.Code, resp.Body.String())
	}

	// ...nor a reserved stdlib name.
	resp = do(t, r, http.MethodPost, "/api/v1/orgs", []byte(`{"name":"net"}`), basic("alice"))
	if resp.Code != http.StatusConflict || errorCode(t, resp) != "name_reserved" {
		t.Fatalf("org named net: expected 409 name_reserved, got %d: %s", resp.Code, resp.Body.String())
	}

	// ...and a username cannot take an organization name.
	resp = do(t, r, http.MethodPost, "/api/v1/orgs", []byte(`{"name":"acme"}`), basic("alice"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("create org: %d %s", resp.Code, resp.Body.String())
	}

	body := `{"username":"acme","email":"acme@example.com","password":"super-secret"}`
	resp = do(t, r, http.MethodPost, "/api/v1/signup", []byte(body), map[string]string{"Content-Type": "application/json"})
	if resp.Code != http.StatusConflict || errorCode(t, resp) != "username_taken" {
		t.Fatalf("signup acme: expected 409 username_taken, got %d: %s", resp.Code, resp.Body.String())
	}

	// Unknown org answers org_not_found.
	resp = do(t, r, http.MethodGet, "/api/v1/orgs/ghost", nil, nil)
	if resp.Code != http.StatusNotFound || errorCode(t, resp) != "org_not_found" {
		t.Fatalf("ghost org: expected 404 org_not_found, got %d", resp.Code)
	}
}
