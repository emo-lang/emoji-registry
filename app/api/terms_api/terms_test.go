package terms_api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIndexActionRendersTerms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	Routes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected a text/html content type, got %q", ct)
	}

	body := w.Body.String()
	for _, want := range []string{
		"<title>Terms of Service</title>",
		"Acceptable Use",
		"Copyright Infringement",
		"will be locked",
		`href="/terms"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %q", want, body)
		}
	}
}
