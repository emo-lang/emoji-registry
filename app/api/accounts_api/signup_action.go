package accounts_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// SignupAction handles POST /api/v1/signup. It creates the account but does
// not log the user in.
func SignupAction(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with username, email and password")
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)

	if !emoji.ValidNamePart(input.Username) {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "username must be 1-64 chars of [a-z0-9_-]")
		return
	}
	if emoji.IsReserved(input.Username) {
		respondError(c, http.StatusForbidden, "name_reserved", input.Username+" is reserved for the official stdlib")
		return
	}
	if !strings.Contains(input.Email, "@") || len(input.Email) > 255 {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "a valid email is required")
		return
	}
	if len(input.Password) < 8 {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "password must be at least 8 characters")
		return
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"username": input.Username}); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	} else if exists {
		respondError(c, http.StatusConflict, "username_taken", "username "+input.Username+" is taken")
		return
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"email": input.Email}); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	} else if exists {
		respondError(c, http.StatusConflict, "email_taken", "email "+input.Email+" is already registered")
		return
	}

	digest, err := utils.EncryptPassword(input.Password)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	user, err := repo.CreateFrom[models.User](sql.H{
		"username":        input.Username,
		"email":           input.Email,
		"password_digest": digest,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"username": user.Username,
		"email":    user.Email,
	})
}
