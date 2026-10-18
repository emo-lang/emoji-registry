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
	"time"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
	"github.com/emo-lang/emoji-registry/app/services/totp"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupLoginTest(t *testing.T) *gin.Engine {
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
	r.POST("/api/v1/login", LoginAction)
	return r
}

func createLoginUser(t *testing.T, enabled bool) (*models.User, string) {
	t.Helper()

	user, err := accounts.Signup("alice", "alice@example.com", "super-secret")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	secret, err := accounts.EnsureTOTPSecret(user)
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	if enabled {
		if err := repo.UpdateByID[models.User](user.ID, sql.H{"totp_enabled": true}); err != nil {
			t.Fatalf("enable: %v", err)
		}
		user.TOTPEnabled = true
	}
	return user, secret
}

func postLogin(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func loginErrorCode(t *testing.T, resp *httptest.ResponseRecorder) string {
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

func TestJSONLoginRequiresEnrollment(t *testing.T) {
	r := setupLoginTest(t)
	createLoginUser(t, false)

	resp := postLogin(t, r, `{"email":"alice@example.com","password":"super-secret"}`)
	if resp.Code != http.StatusForbidden || loginErrorCode(t, resp) != "totp_setup_required" {
		t.Fatalf("expected 403 totp_setup_required, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(resp.Result().Cookies()) != 0 {
		t.Fatalf("expected no session cookie")
	}
}

func TestJSONLoginChallengeMatrix(t *testing.T) {
	r := setupLoginTest(t)
	_, secret := createLoginUser(t, true)

	resp := postLogin(t, r, `{"email":"alice@example.com","password":"super-secret"}`)
	if resp.Code != http.StatusUnauthorized || loginErrorCode(t, resp) != "totp_required" {
		t.Fatalf("missing code: expected 401 totp_required, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = postLogin(t, r, `{"email":"alice@example.com","password":"super-secret","code":"000000"}`)
	if resp.Code != http.StatusUnauthorized || loginErrorCode(t, resp) != "invalid_code" {
		t.Fatalf("wrong code: expected 401 invalid_code, got %d: %s", resp.Code, resp.Body.String())
	}

	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	resp = postLogin(t, r, `{"email":"ALICE@example.com","password":"super-secret","code":"`+code+`"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("valid code: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var found bool
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == middlewares.SessionCookie && cookie.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a session cookie")
	}
}

func TestJSONLoginAcceptsRecoveryCode(t *testing.T) {
	r := setupLoginTest(t)
	user, _ := createLoginUser(t, true)

	codes, err := accounts.GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if err := accounts.StoreRecoveryCodes(user.ID, codes); err != nil {
		t.Fatalf("StoreRecoveryCodes: %v", err)
	}

	resp := postLogin(t, r, `{"email":"alice@example.com","password":"super-secret","code":"`+codes[0]+`"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("recovery code: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	resp = postLogin(t, r, `{"email":"alice@example.com","password":"super-secret","code":"`+codes[0]+`"}`)
	if resp.Code != http.StatusUnauthorized || loginErrorCode(t, resp) != "invalid_code" {
		t.Fatalf("spent code: expected 401 invalid_code, got %d: %s", resp.Code, resp.Body.String())
	}
}
