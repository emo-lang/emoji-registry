package middlewares

import (
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
)

// SessionCookie carries the web session token.
const SessionCookie = "emo_session"

// SessionTTL is how long a web session stays valid.
const SessionTTL = 30 * 24 * time.Hour

// LoadSession populates current_user from the emo_session cookie when it
// holds a valid, unexpired session token. It never aborts the request.
func LoadSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(SessionCookie)
		if err == nil && cookie != "" {
			if session, err := repo.FindOneBy[models.Session](sql.H{"token": cookie}); err == nil && session != nil {
				if session.ExpiresAt.After(time.Now()) {
					if user, err := repo.FindByID[models.User](session.UserID); err == nil && user != nil {
						c.Set("current_user", user)
					}
				}
			}
		}
		c.Next()
	}
}

// RequireWebAuth demands an authenticated web session. API requests get a
// 401 in the shared error shape; page requests are redirected to /login.
func RequireWebAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c) == nil {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				abortUnauthorized(c, "login required")
			} else {
				c.Redirect(http.StatusFound, "/login")
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// CreateSession stores a new session for user and returns its token.
func CreateSession(user *models.User) (*models.Session, error) {
	return repo.CreateFrom[models.Session](sql.H{
		"token":      utils.RandomHex(32),
		"user_id":    user.ID,
		"expires_at": time.Now().Add(SessionTTL),
	})
}
