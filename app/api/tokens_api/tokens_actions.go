package tokens_api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/tokens"
)

var validScopes = map[string]bool{"push": true, "yank": true, "read": true}

// IndexAction renders the token list and create form. Requires a web session
// (enforced by the route group middleware).
func IndexAction(c *gin.Context) {
	renderIndex(c, "")
}

// CreateAction processes the create-token form and shows the plaintext token
// exactly once.
func CreateAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	name := strings.TrimSpace(c.PostForm("name"))
	scopes := c.PostFormArray("scopes")
	expiresInDays := strings.TrimSpace(c.PostForm("expires_in_days"))

	if name == "" || len(name) > 64 {
		renderIndex(c, "Token name must be 1-64 characters.")
		return
	}

	cleanScopes := []string{}
	for _, scope := range scopes {
		if !validScopes[scope] {
			renderIndex(c, "Unknown scope "+scope+".")
			return
		}
		cleanScopes = append(cleanScopes, scope)
	}
	if len(cleanScopes) == 0 {
		renderIndex(c, "Select at least one scope.")
		return
	}

	vals := sql.H{
		"user_id":    user.ID,
		"name":       name,
		"token_hash": "",
		"scopes":     strings.Join(cleanScopes, ","),
	}

	if expiresInDays != "" {
		days, err := strconv.Atoi(expiresInDays)
		if err != nil || days <= 0 {
			renderIndex(c, "Expires in days must be a positive number.")
			return
		}
		vals["expires_at"] = time.Now().Add(time.Duration(days) * 24 * time.Hour)
	}

	plain := middlewares.TokenPrefix + utils.RandomHex(48)
	vals["token_hash"] = middlewares.TokenHash(plain)

	if _, err := repo.CreateFrom[models.APIToken](vals); err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, tokens.Created(user.Username, user.Admin, name, plain))
}

// DeleteAction revokes one of the current user's tokens.
func DeleteAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err == nil {
		if token, err := repo.FindByID[models.APIToken](sql.IdType(id)); err == nil && token != nil && token.UserID == user.ID {
			_ = repo.DeleteByID[models.APIToken](token.ID)
		}
	}

	c.Redirect(http.StatusFound, "/tokens")
}

func renderIndex(c *gin.Context, errMsg string) {
	user := middlewares.CurrentUser(c)

	list, err := repo.FindBy[models.APIToken](sql.H{"user_id": user.ID})
	if err != nil {
		render.Error(c, err)
		return
	}

	status := http.StatusOK
	if errMsg != "" {
		status = http.StatusUnprocessableEntity
	}

	render.HTMLStatus(c, status, tokens.Index(user.Username, user.Admin, list, errMsg))
}
