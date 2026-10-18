package twofa_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/api/auth_api"
	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/services/totp"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

var recoveryCodePattern = regexp.MustCompile(`[A-Z2-7]{5}-[A-Z2-7]{5}`)

func setupTwoFATest(t *testing.T) *gin.Engine {
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
	r.Use(middlewares.RequireTwoFactor())

	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "home") })
	r.GET("/tokens", func(c *gin.Context) { c.String(http.StatusOK, "tokens") })
	r.POST("/signup", auth_api.SignupAction)
	r.POST("/login", auth_api.LoginAction)
	r.POST("/logout", auth_api.LogoutAction)
	r.GET("/api/v1/tokens", func(c *gin.Context) { c.String(http.StatusOK, "api") })

	r.GET("/account/2fa/setup", middlewares.RequirePendingTwoFactor(), ShowSetupAction)
	r.POST("/account/2fa/setup", middlewares.RequirePendingTwoFactor(), SetupAction)
	r.GET("/login/2fa", middlewares.RequirePendingTwoFactor(), ShowChallengeAction)
	r.POST("/login/2fa", middlewares.RequirePendingTwoFactor(), ChallengeAction)

	account := r.Group("/account", middlewares.RequireWebAuth())
	{
		account.GET("/2fa", SecurityAction)
		account.POST("/2fa/recovery-codes", RegenerateRecoveryCodesAction)
	}
	return r
}

func request(t *testing.T, r *gin.Engine, method, target string, form url.Values, token string) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: middlewares.SessionCookie, Value: token})
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func sessionCookie(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()

	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == middlewares.SessionCookie && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatalf("expected a %s cookie", middlewares.SessionCookie)
	return ""
}

func signup(t *testing.T, r *gin.Engine) string {
	t.Helper()

	resp := request(t, r, http.MethodPost, "/signup", url.Values{
		"username": {"alice"},
		"email":    {"alice@example.com"},
		"password": {"super-secret"},
	}, "")
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != "/account/2fa/setup" {
		t.Fatalf("signup: expected 302 to setup, got %d %q: %s", resp.Code, resp.Header().Get("Location"), resp.Body.String())
	}
	return sessionCookie(t, resp)
}

func parseSecret(t *testing.T, body string) string {
	t.Helper()

	const prefix = `<pre class="er-code totp-secret">`
	start := strings.Index(body, prefix)
	if start < 0 {
		t.Fatalf("expected the setup page to show the secret, got %s", body)
	}
	start += len(prefix)
	end := strings.Index(body[start:], "</pre>")
	if end < 0 {
		t.Fatalf("unterminated secret block")
	}
	return strings.TrimSpace(body[start : start+end])
}

func TestSignupRequiresEnrollmentBeforeAnythingElse(t *testing.T) {
	r := setupTwoFATest(t)
	token := signup(t, r)

	if resp := request(t, r, http.MethodGet, "/", nil, token); resp.Code != http.StatusFound || resp.Header().Get("Location") != "/account/2fa/setup" {
		t.Fatalf("pending session: expected redirect to setup, got %d %q", resp.Code, resp.Header().Get("Location"))
	}
	if resp := request(t, r, http.MethodGet, "/tokens", nil, token); resp.Code != http.StatusFound {
		t.Fatalf("pending session: expected /tokens to redirect, got %d", resp.Code)
	}

	resp := request(t, r, http.MethodGet, "/api/v1/tokens", nil, token)
	if resp.Code != http.StatusUnauthorized || !strings.Contains(resp.Body.String(), "totp_setup_required") {
		t.Fatalf("pending API call: expected 401 totp_setup_required, got %d: %s", resp.Code, resp.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	req.AddCookie(&http.Cookie{Name: middlewares.SessionCookie, Value: token})
	req.Header.Set("Authorization", "Bearer token")
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("authorization header: expected the request to pass, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestEnrollmentAndChallenge(t *testing.T) {
	r := setupTwoFATest(t)
	pendingToken := signup(t, r)

	resp := request(t, r, http.MethodGet, "/account/2fa/setup", nil, pendingToken)
	if resp.Code != http.StatusOK {
		t.Fatalf("setup page: expected 200, got %d", resp.Code)
	}
	secret := parseSecret(t, resp.Body.String())

	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}

	resp = request(t, r, http.MethodPost, "/account/2fa/setup", url.Values{"code": {code}}, pendingToken)
	if resp.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	codes := recoveryCodePattern.FindAllString(resp.Body.String(), -1)
	if len(codes) != 10 {
		t.Fatalf("expected 10 recovery codes, got %d: %s", len(codes), resp.Body.String())
	}

	fullToken := sessionCookie(t, resp)
	if fullToken == pendingToken {
		t.Fatalf("expected the session token to be rotated")
	}
	if resp := request(t, r, http.MethodGet, "/", nil, fullToken); resp.Code != http.StatusOK {
		t.Fatalf("authenticated session: expected 200, got %d", resp.Code)
	}

	// Log out, log in again: the challenge is required.
	request(t, r, http.MethodPost, "/logout", nil, fullToken)

	resp = request(t, r, http.MethodPost, "/login", url.Values{
		"email": {"alice@example.com"}, "password": {"super-secret"},
	}, "")
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != "/login/2fa" {
		t.Fatalf("login: expected 302 to challenge, got %d %q", resp.Code, resp.Header().Get("Location"))
	}
	pendingToken = sessionCookie(t, resp)

	code, err = totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	resp = request(t, r, http.MethodPost, "/login/2fa", url.Values{"code": {code}}, pendingToken)
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != "/" {
		t.Fatalf("challenge: expected 302 to /, got %d %q: %s", resp.Code, resp.Header().Get("Location"), resp.Body.String())
	}
	rotated := sessionCookie(t, resp)
	if rotated == pendingToken {
		t.Fatalf("expected the challenge to rotate the session token")
	}

	// A recovery code works once.
	request(t, r, http.MethodPost, "/logout", nil, rotated)
	resp = request(t, r, http.MethodPost, "/login", url.Values{
		"email": {"alice@example.com"}, "password": {"super-secret"},
	}, "")
	pendingToken = sessionCookie(t, resp)

	resp = request(t, r, http.MethodPost, "/login/2fa", url.Values{"code": {strings.ToLower(codes[0])}}, pendingToken)
	if resp.Code != http.StatusFound {
		t.Fatalf("recovery code: expected success, got %d: %s", resp.Code, resp.Body.String())
	}
	fullToken = sessionCookie(t, resp)

	request(t, r, http.MethodPost, "/logout", nil, fullToken)
	resp = request(t, r, http.MethodPost, "/login", url.Values{
		"email": {"alice@example.com"}, "password": {"super-secret"},
	}, "")
	pendingToken = sessionCookie(t, resp)

	resp = request(t, r, http.MethodPost, "/login/2fa", url.Values{"code": {codes[0]}}, pendingToken)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("spent recovery code: expected 422, got %d", resp.Code)
	}
}

func TestInvalidEnrollmentCodeKeepsEnrolling(t *testing.T) {
	r := setupTwoFATest(t)
	pendingToken := signup(t, r)

	resp := request(t, r, http.MethodGet, "/account/2fa/setup", nil, pendingToken)
	secret := parseSecret(t, resp.Body.String())

	resp = request(t, r, http.MethodPost, "/account/2fa/setup", url.Values{"code": {"000000"}}, pendingToken)
	if resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "Invalid code") {
		t.Fatalf("expected 422 invalid code, got %d", resp.Code)
	}
	if got := parseSecret(t, resp.Body.String()); got != secret {
		t.Fatalf("expected the secret to stay stable, got %q then %q", secret, got)
	}
}

func TestRegenerateRecoveryCodes(t *testing.T) {
	r := setupTwoFATest(t)
	pendingToken := signup(t, r)

	resp := request(t, r, http.MethodGet, "/account/2fa/setup", nil, pendingToken)
	secret := parseSecret(t, resp.Body.String())
	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}

	resp = request(t, r, http.MethodPost, "/account/2fa/setup", url.Values{"code": {code}}, pendingToken)
	fullToken := sessionCookie(t, resp)

	if resp := request(t, r, http.MethodGet, "/account/2fa", nil, fullToken); resp.Code != http.StatusOK {
		t.Fatalf("security page: expected 200, got %d", resp.Code)
	}

	resp = request(t, r, http.MethodPost, "/account/2fa/recovery-codes", url.Values{"code": {"000000"}}, fullToken)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong code: expected 422, got %d", resp.Code)
	}

	code, err = totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	resp = request(t, r, http.MethodPost, "/account/2fa/recovery-codes", url.Values{"code": {code}}, fullToken)
	if resp.Code != http.StatusOK {
		t.Fatalf("regenerate: expected 200, got %d", resp.Code)
	}
	if codes := recoveryCodePattern.FindAllString(resp.Body.String(), -1); len(codes) != 10 {
		t.Fatalf("expected 10 new codes, got %d", len(codes))
	}
}
