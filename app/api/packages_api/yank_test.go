package packages_api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

// TestWebYankVersion covers the package page's per-version yank button: only
// the owning scope sees it, and posting it soft-deletes the version.
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

	// The owner sees a yank action on the version row.
	req := httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("owner page: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/p/alice/tools/versions/1.0.0/yank") {
		t.Fatalf("owner page should offer the yank action: %s", w.Body.String())
	}

	// An anonymous visitor never sees it.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil))
	if strings.Contains(w.Body.String(), "/yank") {
		t.Fatalf("anonymous page must not offer the yank action")
	}

	// Yanking redirects back to the package page and flags the version.
	req = httptest.NewRequest(http.MethodPost, "/p/alice/tools/versions/1.0.0/yank", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/p/alice/tools" {
		t.Fatalf("yank: expected redirect to the package page, got %d %q", w.Code, w.Header().Get("Location"))
	}

	version, err := repo.FindOneBy[models.Version](sql.H{"version": "1.0.0"})
	if err != nil || version == nil {
		t.Fatalf("find version: %v", err)
	}
	if !version.Yanked() {
		t.Fatalf("version should be yanked after the web action")
	}

	// Re-yanking is idempotent and the row now shows the yanked flag.
	req = httptest.NewRequest(http.MethodPost, "/p/alice/tools/versions/1.0.0/yank", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("re-yank: expected 302, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/p/alice/tools", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "yanked-tag") {
		t.Fatalf("page should flag the version as yanked")
	}
}
