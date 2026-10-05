package config

import (
	"github.com/daqing/airway/app/websocket"
	"github.com/daqing/airway/lib/jsbuild"
	"github.com/daqing/airway/lib/openapi"
	"github.com/daqing/airway/lib/plugin"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/api/accounts_api"
	"github.com/emo-lang/emoji-registry/app/api/admin_api"
	"github.com/emo-lang/emoji-registry/app/api/auth_api"
	"github.com/emo-lang/emoji-registry/app/api/downloads_api"
	"github.com/emo-lang/emoji-registry/app/api/files_api"
	"github.com/emo-lang/emoji-registry/app/api/health_api"
	"github.com/emo-lang/emoji-registry/app/api/home_api"
	"github.com/emo-lang/emoji-registry/app/api/openapi_api"
	"github.com/emo-lang/emoji-registry/app/api/orgs_api"
	"github.com/emo-lang/emoji-registry/app/api/packages_api"
	"github.com/emo-lang/emoji-registry/app/api/registry_api"
	"github.com/emo-lang/emoji-registry/app/api/search_api"
	"github.com/emo-lang/emoji-registry/app/api/storage_api"
	"github.com/emo-lang/emoji-registry/app/api/tokens_api"
	"github.com/emo-lang/emoji-registry/app/assets"
	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// The openapi:generate command enumerates the routes of the binary it runs
// in; config owns that table, so it registers itself as the route source.
func init() {
	openapi.RegisterRouteSource(Routes)
}

// Routes registers every route — public and internal — at the root paths. This
// is the full router used when the app is served without a URL_PREFIX.
func Routes(r *gin.Engine) {
	PublicRoutes(r)
	HealthRoutes(r)
}

// PublicRoutes registers the user-facing routes: the home page, the WebSocket,
// and the API. When a URL_PREFIX is configured these answer only under the
// prefix; see App.Handler.
func PublicRoutes(r *gin.Engine) {
	r.Use(middlewares.LoadSession())

	r.GET("/", home_api.IndexAction)
	r.GET("/search", middlewares.LimitSearch(), search_api.SearchAction)
	r.GET("/p/:owner/:name", packages_api.ShowAction)
	r.POST("/p/:owner/:name/visibility", middlewares.RequireWebAuth(), packages_api.SetVisibilityAction)

	r.GET("/signup", auth_api.ShowSignupAction)
	r.POST("/signup", middlewares.LimitAuthPage(), auth_api.SignupAction)
	r.GET("/login", auth_api.ShowLoginAction)
	r.POST("/login", middlewares.LimitAuthPage(), auth_api.LoginAction)
	r.POST("/logout", auth_api.LogoutAction)

	orgs_api.WebRoutes(r)
	admin_api.Routes(r)

	tokens := r.Group("/tokens", middlewares.RequireWebAuth())
	{
		tokens.GET("", tokens_api.IndexAction)
		tokens.POST("", tokens_api.CreateAction)
		tokens.POST("/:id/delete", tokens_api.DeleteAction)
	}

	assetRoutes(r)
	websocketRoutes(r)
	apiGroupRoutes(r)
	openapiRoutes(r)
	downloads_api.Routes(r)

	// Protocol A's param routes sit at the root; Gin resolves static
	// segments (the ones above) before params, so they only see package
	// paths. They register last to keep the router tree unambiguous.
	files_api.Routes(r)

	plugin.MountAll(r)
}

// HealthRoutes registers the internal health-check route. It stays reachable at
// the unprefixed root (for load-balancer probes) even when the public routes
// are served under a URL_PREFIX.
func HealthRoutes(r *gin.Engine) {
	health_api.Routes(r)
}

func apiGroupRoutes(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	{
		storage_api.Routes(v1)
		registry_api.Routes(v1)
		accounts_api.Routes(v1)
		orgs_api.APIRoutes(v1)
	}
}

func websocketRoutes(r *gin.Engine) {
	r.GET("/ws", websocket.Conn)
	r.POST("/ws/publish", websocket.Publish)
}

// openapiRoutes serves the generated OpenAPI document at /openapi.json.
func openapiRoutes(r *gin.Engine) {
	openapi_api.Routes(r)
}

// assetRoutes serves the frontend bundle. In local development an in-memory
// esbuild server (started from main when AIRWAY_ENV=local) serves rebuilt
// output directly; otherwise the embedded production bundle answers.
func assetRoutes(r *gin.Engine) {
	if utils.AppConfig().IsLocal {
		if dev := jsbuild.Default(); dev != nil {
			r.GET("/assets/*path", gin.WrapH(dev.Handler()))
			return
		}
	}
	r.GET("/assets/*path", gin.WrapH(assets.Handler()))
}
