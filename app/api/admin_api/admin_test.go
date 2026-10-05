package admin_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
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

	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middlewares.LoadSession())
	Routes(r)
	return r
}

func makeUser(t *testing.T, username string, admin bool) *models.Session {
	t.Helper()

	user, err := repo.CreateFrom[models.User](sql.H{
		"username": username, "email": username + "@example.com",
		"password_digest": "x", "admin": admin,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := middlewares.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func do(t *testing.T, r *gin.Engine, method, target string, form url.Values, session *models.Session) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if session != nil {
		req.AddCookie(&http.Cookie{Name: middlewares.SessionCookie, Value: session.Token})
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func TestRequireAdmin(t *testing.T) {
	r := setupTest(t)

	// Anonymous: 404.
	if resp := do(t, r, http.MethodGet, "/admin/reserved", nil, nil); resp.Code != http.StatusNotFound {
		t.Fatalf("anonymous: expected 404, got %d", resp.Code)
	}

	// Non-admin member: 404.
	member := makeUser(t, "bob", false)
	if resp := do(t, r, http.MethodGet, "/admin/reserved", nil, member); resp.Code != http.StatusNotFound {
		t.Fatalf("member: expected 404, got %d", resp.Code)
	}

	// Admin: 200.
	admin := makeUser(t, "alice", true)
	resp := do(t, r, http.MethodGet, "/admin/reserved", nil, admin)
	if resp.Code != http.StatusOK {
		t.Fatalf("admin: expected 200, got %d", resp.Code)
	}
	// The built-in list renders read-only (no delete forms next to it).
	if !strings.Contains(resp.Body.String(), "Built-in (stdlib)") || !strings.Contains(resp.Body.String(), ">net<") {
		t.Fatalf("expected the built-in list on the page")
	}
}

func TestReservedAddIdempotentDelete(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	add := func(reason string) *httptest.ResponseRecorder {
		return do(t, r, http.MethodPost, "/admin/reserved", url.Values{"name": {"foobar"}, "reason": {reason}}, admin)
	}

	if resp := add("first"); resp.Code != http.StatusFound {
		t.Fatalf("add: expected 302, got %d", resp.Code)
	}
	// Idempotent: re-adding updates the reason.
	if resp := add("second"); resp.Code != http.StatusFound {
		t.Fatalf("re-add: expected 302, got %d", resp.Code)
	}

	rows, err := repo.FindBy[models.ReservedName](sql.H{"name": "foobar"})
	if err != nil || len(rows) != 1 || rows[0].Reason != "second" {
		t.Fatalf("expected one row with updated reason, got %+v", rows)
	}

	// Invalid names are rejected with the form re-rendered.
	resp := do(t, r, http.MethodPost, "/admin/reserved", url.Values{"name": {"BAD NAME"}, "reason": {"x"}}, admin)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid name: expected 422, got %d", resp.Code)
	}

	// Delete.
	resp = do(t, r, http.MethodPost, "/admin/reserved/"+strconv.FormatInt(int64(rows[0].ID), 10)+"/delete", nil, admin)
	if resp.Code != http.StatusFound {
		t.Fatalf("delete: expected 302, got %d", resp.Code)
	}
	if exists, _ := repo.ExistsWhere[models.ReservedName](sql.H{"name": "foobar"}); exists {
		t.Fatalf("expected foobar to be gone")
	}
}

func TestBuiltinNamesHaveNoDeleteButton(t *testing.T) {
	r := setupTest(t)
	admin := makeUser(t, "alice", true)

	resp := do(t, r, http.MethodGet, "/admin/reserved", nil, admin)
	body := resp.Body.String()

	// 20 built-in chips, zero delete forms while nothing is managed.
	if got := strings.Count(body, `class="chip"`); got != 20 {
		t.Fatalf("expected 20 built-in chips, got %d", got)
	}
	if strings.Contains(body, "/delete") {
		t.Fatalf("built-in names must not offer delete")
	}
}

// TestGrantSemantics covers the model-layer logic behind `admin:grant`:
// promote by username, flag readable afterwards.
func TestGrantSemantics(t *testing.T) {
	setupTest(t)

	session := makeUser(t, "carol", false)
	user, _ := repo.FindOneBy[models.User](sql.H{"username": "carol"})
	if user.Admin {
		t.Fatalf("new users must not be admin")
	}

	if err := repo.UpdateByID[models.User](user.ID, sql.H{"admin": true}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	user, _ = repo.FindOneBy[models.User](sql.H{"username": "carol"})
	if !user.Admin {
		t.Fatalf("expected admin flag after grant")
	}

	// The granted admin passes RequireAdmin.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middlewares.LoadSession())
	Routes(r)
	if resp := do(t, r, http.MethodGet, "/admin/reserved", nil, session); resp.Code != http.StatusOK {
		t.Fatalf("granted admin: expected 200, got %d", resp.Code)
	}
}
