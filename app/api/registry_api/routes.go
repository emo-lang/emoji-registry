package registry_api

import (
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// Routes registers the protocol B JSON API (docs/design.md section 5) on an
// API router group mounted at /api/v1.
func Routes(r *gin.RouterGroup) {
	r.POST("/packages", middlewares.TokenAuth("push"), middlewares.LimitPublish(), PublishAction)
	r.GET("/packages/:owner/:name", ShowPackageAction)
	r.PATCH("/packages/:owner/:name", middlewares.TokenAuth("push"), UpdatePackageAction)
	r.GET("/packages/:owner/:name/versions", ListVersionsAction)
	r.DELETE("/packages/:owner/:name/versions/:version", middlewares.TokenAuth("yank"), YankAction)
	r.GET("/dependencies", DependenciesAction)
}
