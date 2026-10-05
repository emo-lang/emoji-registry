package middlewares

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
)

// TokenPrefix prefixes every API token handed out to users.
const TokenPrefix = "emo_"

// TokenHash computes the stored SHA-256 hex digest of a plaintext API token.
func TokenHash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// CurrentUser returns the authenticated user set by TokenAuth or the web
// session middleware, or nil.
func CurrentUser(c *gin.Context) *models.User {
	value, ok := c.Get("current_user")
	if !ok {
		return nil
	}
	user, _ := value.(*models.User)
	return user
}

// CurrentToken returns the API token used for the request, or nil.
func CurrentToken(c *gin.Context) *models.APIToken {
	value, ok := c.Get("current_token")
	if !ok {
		return nil
	}
	token, _ := value.(*models.APIToken)
	return token
}

// TokenAuth authenticates requests carrying an `Authorization: Bearer
// emo_<hex>` API token. Every scope in scopes must be present on the token.
// Failures abort with the shared error shape from docs/design.md section 5.
func TokenAuth(scopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		plain, found := strings.CutPrefix(header, "Bearer ")
		if !found || !strings.HasPrefix(plain, TokenPrefix) {
			abortUnauthorized(c, "a valid Bearer API token is required")
			return
		}

		token, err := repo.FindOneBy[models.APIToken](sql.H{"token_hash": TokenHash(plain)})
		if err != nil {
			abortUnauthorized(c, "token lookup failed")
			return
		}
		if token == nil {
			abortUnauthorized(c, "invalid API token")
			return
		}
		if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
			abortUnauthorized(c, "API token has expired")
			return
		}
		if !tokenHasScopes(token, scopes) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "forbidden",
					"message": "API token lacks the required scope: " + strings.Join(scopes, ", "),
				},
			})
			return
		}

		user, err := repo.FindByID[models.User](token.UserID)
		if err != nil || user == nil {
			abortUnauthorized(c, "token owner no longer exists")
			return
		}

		now := time.Now()
		_ = repo.UpdateByID[models.APIToken](token.ID, sql.H{"last_used_at": now})

		c.Set("current_user", user)
		c.Set("current_token", token)
		c.Next()
	}
}

func tokenHasScopes(token *models.APIToken, required []string) bool {
	held := map[string]bool{}
	for _, scope := range strings.Split(token.Scopes, ",") {
		held[strings.TrimSpace(scope)] = true
	}
	for _, scope := range required {
		if !held[scope] {
			return false
		}
	}
	return true
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "unauthorized",
			"message": message,
		},
	})
}
