package middlewares

import (
	"net/http"

	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/views/errors"
)

// RequireAdmin demands an authenticated admin user. Non-admins — including
// anonymous visitors — get a plain 404 so the admin area's existence stays
// invisible. Mount after LoadSession.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || !user.Admin {
			render.HTMLStatus(c, http.StatusNotFound, errors.NotFound("", "Not found."))
			c.Abort()
			return
		}
		c.Next()
	}
}
