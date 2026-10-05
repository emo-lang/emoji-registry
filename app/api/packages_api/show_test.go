package packages_api

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
	r.GET("/p/:owner/:name", ShowAction)

	return r
}

func publishWithReadme(t *testing.T, r *gin.Engine, readme []byte) {
	t.Helper()

	signup := `{"username":"alice","email":"alice@example.com","password":"super-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", strings.NewReader(signup))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", w.Code, w.Body.String())
	}

	cred := base64.StdEncoding.EncodeToString([]byte("alice@example.com:super-secret"))
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
		"package.emo": []byte("package {\n  name = \"alice/tools\"\n  version = \"1.0.0\"\n}\n"),
		"tools.emo":   []byte("let t = 1"),
	}
	if readme != nil {
		files["README.md"] = readme
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

func TestPackagePageRendersReadme(t *testing.T) {
	r := setupTest(t)

	publishWithReadme(t, r, []byte("# Tools\n\nSome **bold** docs.\n\n<script>alert(1)</script>\n"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<h2>README</h2>") {
		t.Fatalf("expected README section")
	}
	if !strings.Contains(body, "<h1>Tools</h1>") {
		t.Fatalf("expected rendered markdown heading, got: %s", body)
	}
	if !strings.Contains(body, "<strong>bold</strong>") {
		t.Fatalf("expected rendered bold text")
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("raw HTML from README must not be rendered")
	}
}

func TestPackagePageWithoutReadme(t *testing.T) {
	r := setupTest(t)

	publishWithReadme(t, r, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<h2>README</h2>") {
		t.Fatalf("README section must be absent without a README")
	}
}

func TestPrivatePackagePage(t *testing.T) {
	r := setupTest(t)

	publishWithReadme(t, r, nil)

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": "alice", "name": "tools"})
	if err != nil || pkg == nil {
		t.Fatalf("find package: %v", err)
	}
	if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{"visibility": models.VisibilityPrivate}); err != nil {
		t.Fatalf("privatize: %v", err)
	}

	// Anonymous visitors get the 404 page.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("anonymous: expected 404, got %d", w.Code)
	}

	// The owner with a web session sees the page, with the Private badge.
	user, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	session, err := middlewares.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil)
	req.AddCookie(&http.Cookie{Name: middlewares.SessionCookie, Value: session.Token})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("owner: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Private") {
		t.Fatalf("expected the Private badge")
	}
}
