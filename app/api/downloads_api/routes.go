package downloads_api

import (
	"github.com/gin-gonic/gin"
)

// Routes registers the archive download endpoint (docs/design.md B.3). The
// whole <owner>--<name>--<version>.emoji file name arrives as one param
// because Gin params cannot span a static separator inside a segment.
func Routes(r *gin.Engine) {
	r.GET("/downloads/:file", DownloadAction)
}
