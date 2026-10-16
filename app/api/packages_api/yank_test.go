package packages_api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

// TestWebYankVersion covers the package page's per-version yank button: only
// the owning scope sees it, and it only takes effect when the confirmation
// phrase is sent verbatim.
func TestWebYankVersion(t *testing.T) {
	r := setupTest(t)
	publishWithReadme(t, r, nil)

	user, err := repo.FindOneBy[models.User](sql.H{"username": "alice"})
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	session, err := middlewares.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := &http.Cookie{Name: middlewares.SessionCookie, Value: session.Token}

	get := func(cookie *http.Cookie) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /p/alice/tools: expected 200, got %d", w.Code)
		}
		return w.Body.String()
	}
	yank := func(phrase *string) *httptest.ResponseRecorder {
		t.Helper()
		var body string
		if phrase != nil {
			body = url.Values{"confirm": {*phrase}}.Encode()
		}
		req := httptest.NewRequest(http.MethodPost, "/p/alice/tools/versions/1.0.0/yank", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	version := func() *models.Version {
		t.Helper()
		v, err := repo.FindOneBy[models.Version](sql.H{"version": "1.0.0"})
		if err != nil || v == nil {
			t.Fatalf("find version: %v", err)
		}
		return v
	}

	// The owner gets the yank control and its confirmation dialog.
	owner := get(cookie)
	if !strings.Contains(owner, "data-yank-action") || !strings.Contains(owner, "/p/alice/tools/versions/1.0.0/yank") {
		t.Fatalf("owner page should offer the yank action: %s", owner)
	}
	if !strings.Contains(owner, "id=\"yank-dialog\"") {
		t.Fatalf("owner page should render the confirmation dialog")
	}

	// An anonymous visitor never sees it.
	if anon := get(nil); strings.Contains(anon, "data-yank-action") || strings.Contains(anon, "id=\"yank-dialog\"") {
		t.Fatalf("anonymous page must not offer the yank control")
	}

	// A missing or wrong confirmation phrase leaves the version untouched.
	for _, phrase := range []*string{nil, strPtr("yank it"), strPtr("I want to yank this version ")} {
		w := yank(phrase)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/p/alice/tools" {
			t.Fatalf("guarded yank: expected redirect, got %d %q", w.Code, w.Header().Get("Location"))
		}
		if version().Yanked() {
			t.Fatalf("version must not be yanked without the exact phrase")
		}
	}

	// The exact phrase soft-deletes the version.
	w := yank(strPtr(yankConfirmPhrase))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/p/alice/tools" {
		t.Fatalf("yank: expected redirect to the package page, got %d %q", w.Code, w.Header().Get("Location"))
	}
	if !version().Yanked() {
		t.Fatalf("version should be yanked after the confirmed action")
	}

	// Re-yanking is idempotent and the row now shows the yanked flag.
	if w := yank(strPtr(yankConfirmPhrase)); w.Code != http.StatusFound {
		t.Fatalf("re-yank: expected 302, got %d", w.Code)
	}
	if !strings.Contains(get(cookie), "yanked-tag") {
		t.Fatalf("page should flag the version as yanked")
	}
}

func strPtr(s string) *string { return &s }
