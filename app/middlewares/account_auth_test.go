package middlewares

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupAccountAuthTest(t *testing.T) (*gin.Engine, *models.User) {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))
	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}

	user, err := accounts.Signup("alice", "alice@example.com", "super-secret")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LoadSession())
	r.GET("/account", AccountAuth(), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r, user
}

func basicAuthRequest(t *testing.T, r *gin.Engine, email, password, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.SetBasicAuth(email, password)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	}
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func TestAccountAuthRejectsBasicAuthAfterEnrollment(t *testing.T) {
	r, user := setupAccountAuthTest(t)

	if resp := basicAuthRequest(t, r, "alice@example.com", "super-secret", ""); resp.Code != http.StatusOK {
		t.Fatalf("before enrollment: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	if err := repo.UpdateByID[models.User](user.ID, sql.H{"totp_enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	resp := basicAuthRequest(t, r, "alice@example.com", "super-secret", "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("after enrollment: expected 401, got %d", resp.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil || body.Error.Code != "totp_required" {
		t.Fatalf("expected totp_required, got %s", resp.Body.String())
	}
}

func TestAccountAuthStillAcceptsSessions(t *testing.T) {
	r, user := setupAccountAuthTest(t)

	if err := repo.UpdateByID[models.User](user.ID, sql.H{"totp_enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	session, err := CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if resp := basicAuthRequest(t, r, "", "", session.Token); resp.Code != http.StatusOK {
		t.Fatalf("session: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
}
