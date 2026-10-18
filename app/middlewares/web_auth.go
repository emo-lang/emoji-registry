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

// SessionTTL is how long a fully authenticated web session stays valid.
const SessionTTL = 30 * 24 * time.Hour

// PendingSessionTTL bounds how long a password-only session may stay around
// while the user completes the second factor.
const PendingSessionTTL = 15 * time.Minute

var twoFactorExemptPaths = map[string]bool{
	"/login":             true,
	"/signup":            true,
	"/logout":            true,
	"/login/2fa":         true,
	"/account/2fa/setup": true,
	"/api/v1/login":      true,
	"/api/v1/signup":     true,
	"/ws":                true,
	"/openapi.json":      true,
	"/health":            true,
	"/admin/login":       true,
}

// LoadSession populates current_user from the emo_session cookie when it
// holds a valid, unexpired session token. Sessions that still need the
// second factor are exposed separately and never count as authenticated.
// It never aborts the request.
func LoadSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(SessionCookie)
		if err == nil && cookie != "" {
			if session, err := repo.FindOneBy[models.Session](sql.H{"token": cookie}); err == nil && session != nil {
				if session.ExpiresAt.After(time.Now()) {
					if user, err := repo.FindByID[models.User](session.UserID); err == nil && user != nil {
						c.Set("current_session", session)
						if session.TOTPPending {
							c.Set("pending_two_factor_session", session)
							c.Set("pending_two_factor_user", user)
						} else {
							c.Set("current_user", user)
						}
					}
				}
			}
		}
		c.Next()
	}
}

// RequireTwoFactor keeps a password-only session from reaching anything
// except the second-factor pages. Requests with an Authorization header are
// left to the API auth middlewares.
func RequireTwoFactor() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := PendingTwoFactorSession(c)
		if session == nil {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		if twoFactorExemptPaths[path] || strings.HasPrefix(path, "/assets/") {
			c.Next()
			return
		}

		if strings.HasPrefix(path, "/api/") {
			if c.GetHeader("Authorization") != "" {
				c.Next()
				return
			}
			code := "totp_setup_required"
			if user := PendingTwoFactorUser(c); user != nil && user.TOTPEnabled {
				code = "totp_required"
			}
			abortUnauthorizedCode(c, code, "two-factor authentication required")
			return
		}

		target := "/account/2fa/setup"
		if user := PendingTwoFactorUser(c); user != nil && user.TOTPEnabled {
			target = "/login/2fa"
		}
		c.Redirect(http.StatusFound, target)
		c.Abort()
	}
}

// RequirePendingTwoFactor guards the setup and challenge pages.
func RequirePendingTwoFactor() gin.HandlerFunc {
	return func(c *gin.Context) {
		if PendingTwoFactorSession(c) == nil {
			if CurrentUser(c) != nil {
				c.Redirect(http.StatusFound, "/")
			} else {
				c.Redirect(http.StatusFound, "/login")
			}
			c.Abort()
			return
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

// CreateSession stores a new fully authenticated session for user.
func CreateSession(user *models.User) (*models.Session, error) {
	return repo.CreateFrom[models.Session](sql.H{
		"token":      utils.RandomHex(32),
		"user_id":    user.ID,
		"expires_at": time.Now().Add(SessionTTL),
	})
}

// CreatePendingSession stores a short-lived session that still needs the
// second factor. It is upgraded by CompleteTwoFactor.
func CreatePendingSession(user *models.User) (*models.Session, error) {
	return repo.CreateFrom[models.Session](sql.H{
		"token":        utils.RandomHex(32),
		"user_id":      user.ID,
		"totp_pending": true,
		"expires_at":   time.Now().Add(PendingSessionTTL),
	})
}

func PendingTwoFactorUser(c *gin.Context) *models.User {
	value, ok := c.Get("pending_two_factor_user")
	if !ok {
		return nil
	}
	user, _ := value.(*models.User)
	return user
}

func PendingTwoFactorSession(c *gin.Context) *models.Session {
	value, ok := c.Get("pending_two_factor_session")
	if !ok {
		return nil
	}
	session, _ := value.(*models.Session)
	return session
}

// CompleteTwoFactor promotes a pending session to a full session: the token
// is rotated (so the pre-2FA cookie cannot be reused), the TTL returns to
// SessionTTL, and the user becomes current.
func CompleteTwoFactor(c *gin.Context, session *models.Session, user *models.User) error {
	token := utils.RandomHex(32)
	if err := repo.UpdateByID[models.Session](session.ID, sql.H{
		"token":        token,
		"totp_pending": false,
		"expires_at":   time.Now().Add(SessionTTL),
	}); err != nil {
		return err
	}

	session.Token = token
	session.TOTPPending = false
	session.ExpiresAt = time.Now().Add(SessionTTL)
	c.Set("current_session", session)
	c.Set("current_user", user)
	c.SetCookie(SessionCookie, token, int(SessionTTL.Seconds()), "/", "", false, true)
	return nil
}

func SetSessionCookie(c *gin.Context, session *models.Session) {
	c.SetCookie(SessionCookie, session.Token, int(session.ExpiresAt.Sub(time.Now()).Seconds()), "/", "", false, true)
}
