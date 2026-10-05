package search_api

import (
	"fmt"
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

func setupTest(t *testing.T) *gin.Engine {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))

	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}

	for i := 1; i <= 25; i++ {
		if _, err := repo.CreateFrom[models.Package](sql.H{
			"owner_scope": "alice",
			"name":        fmt.Sprintf("pkg%02d", i),
			"description": "a test package",
			"license":     "",
			"homepage":    "",
			"repository":  "",
			"user_id":     0,
			"downloads":   0,
		}); err != nil {
			t.Fatalf("seed package: %v", err)
		}
	}

	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/search", SearchAction)
	return r
}

func get(t *testing.T, r *gin.Engine, target string) string {
	t.Helper()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d", target, w.Code)
	}
	return w.Body.String()
}

func TestSearchPaginates(t *testing.T) {
	r := setupTest(t)

	page1 := get(t, r, "/search?q=pkg")
	if !strings.Contains(page1, "25 result(s)") {
		t.Fatalf("expected total count, got: %s", page1)
	}
	if got := strings.Count(page1, `class="pkg-card"`); got != 20 {
		t.Fatalf("page 1 should render 20 cards, got %d", got)
	}
	if !strings.Contains(page1, "Next") || strings.Contains(page1, "Previous") {
		t.Fatalf("page 1 should only have a Next link")
	}

	page2 := get(t, r, "/search?q=pkg&page=2")
	if got := strings.Count(page2, `class="pkg-card"`); got != 5 {
		t.Fatalf("page 2 should render 5 cards, got %d", got)
	}
	if !strings.Contains(page2, "Previous") || strings.Contains(page2, "Next →") {
		t.Fatalf("page 2 should only have a Previous link")
	}

	// No results: the LIKE match is case-insensitive on both sides.
	none := get(t, r, "/search?q=nomatch")
	if !strings.Contains(none, "0 result(s)") {
		t.Fatalf("expected 0 results")
	}

	upper := get(t, r, "/search?q=PKG01")
	if !strings.Contains(upper, "pkg01") {
		t.Fatalf("search should be case-insensitive")
	}

	empty := get(t, r, "/search?q=")
	if !strings.Contains(empty, "Type a query") {
		t.Fatalf("empty query should prompt for keywords")
	}
}

func TestSearchNeverLeaksPrivatePackages(t *testing.T) {
	r := setupTest(t)

	if _, err := repo.CreateFrom[models.Package](sql.H{
		"owner_scope": "alice", "name": "pkgsecret", "description": "hidden",
		"license": "", "homepage": "", "repository": "", "user_id": 0,
		"downloads": 0, "visibility": models.VisibilityPrivate,
	}); err != nil {
		t.Fatalf("seed private package: %v", err)
	}

	body := get(t, r, "/search?q=pkg")
	if strings.Contains(body, "pkgsecret") {
		t.Fatalf("private package leaked into search")
	}
	if !strings.Contains(body, "25 result(s)") {
		t.Fatalf("private package must not count toward totals: %s", body)
	}
}
