package files_api

import (
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// Routes registers the protocol A bare-file compatibility layer
// (docs/design.md section 5) at the root. Gin gives static segments
// (/api, /assets, /downloads, /health, /ws, /openapi.json) priority over
// these params, so the catch-all only ever sees package paths.
//
// The manifest and individual sources share one catch-all route because Gin
// forbids a wildcard next to a static sibling (package.emo) at the same
// position. OptionalAuth makes tokens/sessions available; private packages
// answer 404 to unauthorized requests.
func Routes(r *gin.Engine) {
	r.GET("/:owner/:name/versions", middlewares.OptionalAuth(), VersionsAction)
	r.GET("/:owner/:name/:version/*filepath", middlewares.OptionalAuth(), FileAction)
}
