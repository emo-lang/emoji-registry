package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	airwayredis "github.com/daqing/airway-redis-plugin"
	"github.com/gin-gonic/gin"
)

func TestRateLimitersUseRedisBackendWhenConfigured(t *testing.T) {
	server := miniredis.RunT(t)
	if err := airwayredis.Setup(airwayredis.Config{URL: "redis://" + server.Addr()}); err != nil {
		t.Fatalf("redis setup: %v", err)
	}

	if RateLimitBackendName() != "redis" {
		t.Fatalf("expected redis backend name")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/search", LimitSearch(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 61: expected 429, got %d", w.Code)
	}

	// The auth budget shares the window length (60s) but must not share the
	// counter: a different key namespace keeps it independent.
	r.POST("/api/v1/login", LimitAuthAPI(), func(c *gin.Context) { c.Status(http.StatusOK) })
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("auth budget must be independent of search, got %d", w.Code)
	}

	// The counter lives in Redis under the ratelimit prefix.
	found := false
	for _, key := range server.Keys() {
		if len(key) > len("ratelimit:60:") && key[:len("ratelimit:60:")] == "ratelimit:60:" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a ratelimit:60:* key in redis, got %v", server.Keys())
	}
}
