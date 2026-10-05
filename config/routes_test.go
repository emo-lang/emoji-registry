package config

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRoutesRegistersCoreEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	Routes(r)

	registered := map[string]bool{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	expected := []string{
		"GET /",
		"GET /health",
		"GET /openapi.json",
		"GET /ws",
		"POST /ws/publish",
		"POST /api/v1/storage",
		"GET /api/v1/storage/*key",
		"DELETE /api/v1/storage/*key",
		"POST /api/v1/signup",
		"POST /api/v1/login",
		"POST /api/v1/tokens",
		"GET /api/v1/tokens",
		"DELETE /api/v1/tokens/:id",
		"POST /api/v1/packages",
		"GET /api/v1/packages/:owner/:name",
		"PATCH /api/v1/packages/:owner/:name",
		"GET /api/v1/packages/:owner/:name/versions",
		"DELETE /api/v1/packages/:owner/:name/versions/:version",
		"GET /api/v1/dependencies",
		"GET /downloads/:file",
		"GET /:owner/:name/versions",
		"GET /:owner/:name/:version/*filepath",
		"GET /search",
		"GET /p/:owner/:name",
		"GET /signup",
		"POST /signup",
		"GET /login",
		"POST /login",
		"POST /logout",
		"GET /tokens",
		"POST /tokens",
		"POST /tokens/:id/delete",
	}

	for _, route := range expected {
		if !registered[route] {
			t.Fatalf("expected route %s to be registered, got %#v", route, registered)
		}
	}
}
