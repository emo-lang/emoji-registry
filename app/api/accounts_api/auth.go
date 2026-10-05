package accounts_api

import (
	"net/http"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// accountAuth accepts either a valid web session cookie or HTTP basic auth
// with email + password (the CLI uses the latter to mint tokens).
func accountAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user := middlewares.CurrentUser(c); user != nil {
			c.Next()
			return
		}

		email, password, ok := c.Request.BasicAuth()
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized", "session cookie or basic auth required")
			c.Abort()
			return
		}

		user, err := repo.FindOneBy[models.User](sql.H{"email": email})
		if err != nil || user == nil || !utils.ComparePassword(utils.PasswordDigest(user.PasswordDigest), password) {
			respondError(c, http.StatusUnauthorized, "unauthorized", "invalid email or password")
			c.Abort()
			return
		}

		c.Set("current_user", user)
		c.Next()
	}
}
