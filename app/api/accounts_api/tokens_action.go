package accounts_api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

var validScopes = map[string]bool{"push": true, "yank": true, "read": true}

// CreateTokenAction handles POST /api/v1/tokens. The plaintext token is
// returned exactly once; only its SHA-256 hash is stored.
func CreateTokenAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	var input struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with name and scopes")
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 64 {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "name must be 1-64 characters")
		return
	}
	if len(input.Scopes) == 0 {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "at least one scope is required")
		return
	}
	for _, scope := range input.Scopes {
		if !validScopes[scope] {
			respondError(c, http.StatusUnprocessableEntity, "invalid_params", "unknown scope "+scope)
			return
		}
	}
	if input.ExpiresInDays < 0 {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "expires_in_days must not be negative")
		return
	}

	plain := middlewares.TokenPrefix + utils.RandomHex(48)

	vals := sql.H{
		"user_id":    user.ID,
		"name":       input.Name,
		"token_hash": middlewares.TokenHash(plain),
		"scopes":     strings.Join(input.Scopes, ","),
	}
	var expiresAt *time.Time
	if input.ExpiresInDays > 0 {
		t := time.Now().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &t
		vals["expires_at"] = *expiresAt
	}

	token, err := repo.CreateFrom[models.APIToken](vals)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusCreated, tokenObject(token, plain))
}

// ListTokensAction handles GET /api/v1/tokens.
func ListTokensAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	tokens, err := repo.FindBy[models.APIToken](sql.H{"user_id": user.ID})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	list := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		list = append(list, tokenObject(token, ""))
	}

	c.JSON(http.StatusOK, gin.H{"tokens": list})
}

// DeleteTokenAction handles DELETE /api/v1/tokens/:id. Only the token owner
// may revoke it.
func DeleteTokenAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusNotFound, "token_not_found", "no such token")
		return
	}

	token, err := repo.FindByID[models.APIToken](sql.IdType(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if token == nil || token.UserID != user.ID {
		respondError(c, http.StatusNotFound, "token_not_found", "no such token")
		return
	}

	if err := repo.DeleteByID[models.APIToken](token.ID); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": token.ID})
}

func tokenObject(token *models.APIToken, plain string) gin.H {
	out := gin.H{
		"id":           token.ID,
		"name":         token.Name,
		"scopes":       strings.Split(token.Scopes, ","),
		"expires_at":   token.ExpiresAt,
		"last_used_at": token.LastUsedAt,
		"created_at":   token.CreatedAt.UTC().Format(time.RFC3339),
	}
	if plain != "" {
		out["token"] = plain
	}
	return out
}
