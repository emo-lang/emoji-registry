package users_api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/api/accounts_api"
	"github.com/emo-lang/emoji-registry/app/api/registry_api"
	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
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
	r.Use(middlewares.LoadSession())

	v1 := r.Group("/api/v1")
	accounts_api.Routes(v1)
	registry_api.Routes(v1)

	Routes(r)

	return r
}

// publishAs signs up / logs in as username and publishes one package under
// that account's own scope.
func publishAs(t *testing.T, r *gin.Engine, username string) {
	t.Helper()

	signup := `{"username":"` + username + `","email":"` + username + `@example.com","password":"super-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", strings.NewReader(signup))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", w.Code, w.Body.String())
	}

	cred := base64.StdEncoding.EncodeToString([]byte(username + "@example.com:super-secret"))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(`{"name":"cli","scopes":["push"]}`))
	req.Header.Set("Authorization", "Basic "+cred)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var tokenOut struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &tokenOut); err != nil {
		t.Fatalf("token: %v", err)
	}

	files := map[string][]byte{
		"package.emo": []byte("package {\n  name = \"" + username + "/tools\"\n  version = \"1.0.0\"\n}\n"),
		"tools.emo":   []byte("let t = 1"),
	}
	archive, err := emoji.BuildArchive(files)
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/packages", bytes.NewReader(archive))
	req.Header.Set("Authorization", "Bearer "+tokenOut.Token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
}

func TestProfileListsUploadedPackages(t *testing.T) {
	r := setupTest(t)

	publishAs(t, r, "alice")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/u/alice", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		"<title>alice</title>",
		"alice/tools",
		"1 package",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %q", want, body)
		}
	}
}

func TestProfileEmptyForFreshAccount(t *testing.T) {
	r := setupTest(t)

	signup := `{"username":"bob","email":"bob@example.com","password":"super-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", strings.NewReader(signup))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/u/bob", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hasn't published any packages yet") {
		t.Fatalf("expected the empty-state message, got %q", w.Body.String())
	}
}

func TestProfileUnknownUserIs404(t *testing.T) {
	r := setupTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/u/nobody", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProfileHidesPrivatePackages(t *testing.T) {
	r := setupTest(t)

	publishAs(t, r, "alice")

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": "alice", "name": "tools"})
	if err != nil || pkg == nil {
		t.Fatalf("find package: %v", err)
	}
	if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{"visibility": models.VisibilityPrivate}); err != nil {
		t.Fatalf("privatize: %v", err)
	}

	// Anonymous visitors don't see the private package.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/u/alice", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "alice/tools") {
		t.Fatalf("private package must not be listed to anonymous visitors")
	}

	// The owner sees it on their own profile.
	user, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	session, err := middlewares.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/u/alice", nil)
	req.AddCookie(&http.Cookie{Name: middlewares.SessionCookie, Value: session.Token})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("owner: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "alice/tools") {
		t.Fatalf("owner must see their own private package")
	}
}
