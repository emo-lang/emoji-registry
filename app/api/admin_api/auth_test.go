package admin_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// doReq issues a request carrying arbitrary cookies (the env-admin cookie is
// not a session, so the do helper in admin_test.go cannot send it).
func doReq(t *testing.T, r *gin.Engine, method, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

// Without ADMIN_USERNAME/ADMIN_PASSWORD the login form stays invisible: the
// env sign-in path does not exist, matching RequireAdmin's plain 404.
func TestAdminLoginHiddenWithoutEnv(t *testing.T) {
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	r := setupTest(t)

	if resp := doReq(t, r, http.MethodGet, "/admin/login", nil); resp.Code != http.StatusNotFound {
		t.Fatalf("login page: expected 404, got %d", resp.Code)
	}
	if resp := doReq(t, r, http.MethodPost, "/admin/login", url.Values{"username": {"x"}, "password": {"y"}}); resp.Code != http.StatusNotFound {
		t.Fatalf("login post: expected 404, got %d", resp.Code)
	}
}

func TestAdminEnvLoginFlow(t *testing.T) {
	t.Setenv("ADMIN_USERNAME", "root")
	t.Setenv("ADMIN_PASSWORD", "s3cret")
	r := setupTest(t)

	if resp := doReq(t, r, http.MethodGet, "/admin/login", nil); resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "Admin log in") {
		t.Fatalf("login page: expected 200 with the form, got %d", resp.Code)
	}

	// Wrong password re-renders the form without a cookie.
	resp := doReq(t, r, http.MethodPost, "/admin/login", url.Values{"username": {"root"}, "password": {"nope"}})
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong password: expected 422, got %d", resp.Code)
	}
	for _, ck := range resp.Result().Cookies() {
		if ck.Name == middlewares.AdminCookie && ck.Value != "" {
			t.Fatalf("wrong password must not issue an admin cookie")
		}
	}

	// Correct credentials redirect to /admin with the admin cookie.
	resp = doReq(t, r, http.MethodPost, "/admin/login", url.Values{"username": {" root "}, "password": {"s3cret"}})
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != "/admin" {
		t.Fatalf("login: expected 302 to /admin, got %d %s", resp.Code, resp.Header().Get("Location"))
	}
	var adminCookie *http.Cookie
	for _, ck := range resp.Result().Cookies() {
		if ck.Name == middlewares.AdminCookie && ck.Value != "" {
			adminCookie = ck
		}
	}
	if adminCookie == nil {
		t.Fatalf("login must issue the admin cookie")
	}

	// The admin cookie alone — no site session — opens the admin area.
	for _, target := range []string{"/admin", "/admin/reserved"} {
		if resp := doReq(t, r, http.MethodGet, target, nil, adminCookie); resp.Code != http.StatusOK {
			t.Fatalf("GET %s with admin cookie: expected 200, got %d", target, resp.Code)
		}
	}

	// Already signed in: the login form bounces to /admin.
	if resp := doReq(t, r, http.MethodGet, "/admin/login", nil, adminCookie); resp.Code != http.StatusFound {
		t.Fatalf("login page while admin: expected 302, got %d", resp.Code)
	}

	// A tampered signature is rejected like a missing one.
	tampered := adminCookie.Value
	if tampered[len(tampered)-1] == '0' {
		tampered = tampered[:len(tampered)-1] + "1"
	} else {
		tampered = tampered[:len(tampered)-1] + "0"
	}
	bad := &http.Cookie{Name: middlewares.AdminCookie, Value: tampered}
	if resp := doReq(t, r, http.MethodGet, "/admin", nil, bad); resp.Code != http.StatusNotFound {
		t.Fatalf("tampered cookie: expected 404, got %d", resp.Code)
	}

	// A properly signed but expired cookie is rejected too.
	expired := &http.Cookie{Name: middlewares.AdminCookie, Value: middlewares.AdminCookieValue(time.Now().Add(-time.Minute))}
	if resp := doReq(t, r, http.MethodGet, "/admin", nil, expired); resp.Code != http.StatusNotFound {
		t.Fatalf("expired cookie: expected 404, got %d", resp.Code)
	}

	// Logout clears the cookie; the next /admin visit falls back to 404.
	resp = doReq(t, r, http.MethodPost, "/admin/logout", nil, adminCookie)
	if resp.Code != http.StatusFound || resp.Header().Get("Location") != "/admin/login" {
		t.Fatalf("logout: expected 302 to /admin/login, got %d", resp.Code)
	}
	var cleared string
	for _, ck := range resp.Result().Cookies() {
		if ck.Name == middlewares.AdminCookie {
			cleared = ck.Value
		}
	}
	if cleared != "" {
		t.Fatalf("logout must clear the admin cookie, got %q", cleared)
	}
	empty := &http.Cookie{Name: middlewares.AdminCookie, Value: ""}
	if resp := doReq(t, r, http.MethodGet, "/admin", nil, empty); resp.Code != http.StatusNotFound {
		t.Fatalf("after logout: expected 404, got %d", resp.Code)
	}
}
