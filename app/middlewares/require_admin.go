package middlewares

import (
	"net/http"

	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/views/errors"
)

// IsAdmin reports whether the request comes from an admin: a signed-in site
// account with the admin flag (`admin:grant`) or a valid env-credentials
// cookie issued by /admin/login.
func IsAdmin(c *gin.Context) bool {
	if user := CurrentUser(c); user != nil && user.Admin {
		return true
	}
	return CurrentAdmin(c)
}

// RequireAdmin demands an admin — either an `admin:grant` site account or an
// env-credentials session from /admin/login. Everyone else, including
// anonymous visitors, gets a plain 404 so the admin area's existence stays
// invisible. Mount after LoadSession.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsAdmin(c) {
			render.HTMLStatus(c, http.StatusNotFound, errors.NotFound("", "Not found."))
			c.Abort()
			return
		}
		c.Next()
	}
}
