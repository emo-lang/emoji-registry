package accounts_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

// LoginAction handles POST /api/v1/login: email + password in exchange for
// an emo_session cookie.
func LoginAction(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with email and password")
		return
	}

	user, err := repo.FindOneBy[models.User](sql.H{"email": strings.TrimSpace(input.Email)})
	if err != nil || user == nil || !utils.ComparePassword(utils.PasswordDigest(user.PasswordDigest), input.Password) {
		respondError(c, http.StatusUnauthorized, "unauthorized", "invalid email or password")
		return
	}

	session, err := middlewares.CreateSession(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.SetCookie(middlewares.SessionCookie, session.Token, int(middlewares.SessionTTL.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"username": user.Username, "email": user.Email})
}
