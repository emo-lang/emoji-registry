package middlewares

import (
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/access"
)

// OptionalAuth authenticates a Bearer token when one is present and valid,
// setting current_user and current_token like TokenAuth, but never aborts:
// missing or invalid credentials simply leave the request anonymous. Used on
// public read endpoints so private packages can answer 404 — never 401 — to
// avoid leaking their existence.
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		plain, found := strings.CutPrefix(header, "Bearer ")
		if !found || !strings.HasPrefix(plain, TokenPrefix) {
			c.Next()
			return
		}

		token, err := repo.FindOneBy[models.APIToken](sql.H{"token_hash": TokenHash(plain)})
		if err != nil || token == nil {
			c.Next()
			return
		}
		if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
			c.Next()
			return
		}

		user, err := repo.FindByID[models.User](token.UserID)
		if err != nil || user == nil {
			c.Next()
			return
		}

		c.Set("current_user", user)
		c.Set("current_token", token)
		c.Next()
	}
}

// CanReadPackage combines the request's credentials into one read decision
// for pkg. An API token needs the read scope; a web session user needs plain
// read access.
func CanReadPackage(c *gin.Context, pkg *models.Package) bool {
	if !pkg.Private() {
		return true
	}

	if token := CurrentToken(c); token != nil {
		user, err := repo.FindByID[models.User](token.UserID)
		if err != nil || user == nil {
			return false
		}
		return access.TokenCanRead(token, user, pkg)
	}

	if user := CurrentUser(c); user != nil {
		return access.CanRead(user, pkg)
	}

	return false
}
