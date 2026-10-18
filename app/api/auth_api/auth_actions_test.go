package auth_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupAuthTest(t *testing.T) *gin.Engine {
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

	r.POST("/signup", SignupAction)
	r.POST("/login", LoginAction)
	return r
}

func postForm(t *testing.T, r *gin.Engine, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func TestSignupFormRequiresEveryField(t *testing.T) {
	r := setupAuthTest(t)

	cases := []struct {
		name string
		form url.Values
		want string
	}{
		{"missing username", url.Values{"email": {"alice@example.com"}, "password": {"super-secret"}}, "username is required."},
		{"missing email", url.Values{"username": {"alice"}, "password": {"super-secret"}}, "email is required."},
		{"missing password", url.Values{"username": {"alice"}, "email": {"alice@example.com"}}, "password is required."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postForm(t, r, "/signup", tc.form)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d", resp.Code)
			}
			if !strings.Contains(resp.Body.String(), tc.want) {
				t.Fatalf("expected body to contain %q, got %s", tc.want, resp.Body.String())
			}
		})
	}
}

func TestSignupFormRejectsDuplicatesAndNormalizesEmail(t *testing.T) {
	r := setupAuthTest(t)

	resp := postForm(t, r, "/signup", url.Values{
		"username": {"alice"},
		"email":    {"Alice@Example.com"},
		"password": {"super-secret"},
	})
	if resp.Code != http.StatusFound {
		t.Fatalf("signup: expected 302, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = postForm(t, r, "/signup", url.Values{
		"username": {"bob"},
		"email":    {"ALICE@example.com"},
		"password": {"super-secret"},
	})
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "already registered") {
		t.Fatalf("expected 422 already registered, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = postForm(t, r, "/signup", url.Values{
		"username": {"alice"},
		"email":    {"other@example.com"},
		"password": {"super-secret"},
	})
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "taken by a user account") {
		t.Fatalf("expected 422 taken by a user account, got %d: %s", resp.Code, resp.Body.String())
	}

	// Login accepts the address in any case.
	resp = postForm(t, r, "/login", url.Values{
		"email":    {"ALICE@EXAMPLE.COM"},
		"password": {"super-secret"},
	})
	if resp.Code != http.StatusFound {
		t.Fatalf("login: expected 302, got %d: %s", resp.Code, resp.Body.String())
	}
}
