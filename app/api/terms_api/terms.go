package terms_api

import (
	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/views/terms"
)

// Routes registers the public Terms of Service page.
func Routes(r *gin.Engine) {
	r.GET("/terms", IndexAction)
}

// IndexAction handles GET /terms.
func IndexAction(c *gin.Context) {
	username, admin := "", false
	if user := middlewares.CurrentUser(c); user != nil {
		username, admin = user.Username, user.Admin
	}

	render.HTML(c, terms.Index(username, admin))
}
