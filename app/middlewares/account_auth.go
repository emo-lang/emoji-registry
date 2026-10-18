package middlewares

import (
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
)

// AccountAuth accepts either a valid web session cookie or HTTP basic auth
// with email + password (the CLI uses the latter for account operations).
func AccountAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user := CurrentUser(c); user != nil {
			c.Next()
			return
		}

		email, password, ok := c.Request.BasicAuth()
		if !ok {
			abortUnauthorized(c, "session cookie or basic auth required")
			return
		}

		user, err := repo.FindOneBy[models.User](sql.H{"email": accounts.NormalizeEmail(email)})
		if err != nil || user == nil || !utils.ComparePassword(utils.PasswordDigest(user.PasswordDigest), password) {
			abortUnauthorized(c, "invalid email or password")
			return
		}

		if user.TOTPEnabled {
			abortUnauthorizedCode(c, "totp_required", "two-factor authentication is enabled for this account; create an API token from the web UI")
			return
		}

		c.Set("current_user", user)
		c.Next()
	}
}
