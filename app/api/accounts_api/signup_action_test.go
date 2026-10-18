package accounts_api

import (
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
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupSignupTest(t *testing.T) *gin.Engine {
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
	r.POST("/api/v1/signup", SignupAction)
	return r
}

func postSignup(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func signupErrorCode(t *testing.T, resp *httptest.ResponseRecorder) string {
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

func TestSignupRequiresEveryField(t *testing.T) {
	r := setupSignupTest(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing username", `{"email":"alice@example.com","password":"super-secret"}`, "username is required"},
		{"missing email", `{"username":"alice","password":"super-secret"}`, "email is required"},
		{"missing password", `{"username":"alice","email":"alice@example.com"}`, "password is required"},
		{"whitespace password", `{"username":"alice","email":"alice@example.com","password":"        "}`, "password is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postSignup(t, r, tc.body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), tc.want) {
				t.Fatalf("expected body to contain %q, got %s", tc.want, resp.Body.String())
			}
		})
	}
}

func TestSignupRejectsDuplicateUsernameAndEmail(t *testing.T) {
	r := setupSignupTest(t)

	resp := postSignup(t, r, `{"username":"alice","email":"Alice@Example.com","password":"super-secret"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", resp.Code, resp.Body.String())
	}

	alice, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || alice == nil {
		t.Fatalf("find alice: %v", err)
	}
	if alice.Email != "alice@example.com" {
		t.Fatalf("expected normalized email, got %q", alice.Email)
	}

	// Addresses differing only in case are the same account.
	resp = postSignup(t, r, `{"username":"bob","email":"ALICE@example.com","password":"super-secret"}`)
	if resp.Code != http.StatusConflict || signupErrorCode(t, resp) != "email_taken" {
		t.Fatalf("expected 409 email_taken, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = postSignup(t, r, `{"username":"alice","email":"other@example.com","password":"super-secret"}`)
	if resp.Code != http.StatusConflict || signupErrorCode(t, resp) != "username_taken" {
		t.Fatalf("expected 409 username_taken, got %d: %s", resp.Code, resp.Body.String())
	}

	// Usernames and organizations share one namespace.
	if _, err := repo.CreateFrom[models.Organization](sql.H{
		"name":         "acme",
		"display_name": "Acme",
		"user_id":      alice.ID,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}

	resp = postSignup(t, r, `{"username":"acme","email":"acme@example.com","password":"super-secret"}`)
	if resp.Code != http.StatusConflict || signupErrorCode(t, resp) != "username_taken" {
		t.Fatalf("expected 409 username_taken, got %d: %s", resp.Code, resp.Body.String())
	}
}
