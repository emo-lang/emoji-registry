package accounts_api

import (
	"net/http"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
)

// LoginAction handles POST /api/v1/login: email + password in exchange for
// an emo_session cookie.
func LoginAction(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with email and password")
		return
	}

	user, err := repo.FindOneBy[models.User](sql.H{"email": accounts.NormalizeEmail(input.Email)})
	if err != nil || user == nil || !utils.ComparePassword(utils.PasswordDigest(user.PasswordDigest), input.Password) {
		respondError(c, http.StatusUnauthorized, "unauthorized", "invalid email or password")
		return
	}

	if !user.TOTPEnabled {
		respondError(c, http.StatusForbidden, "totp_setup_required", "two-factor authentication must be set up in the web UI before logging in")
		return
	}

	if input.Code == "" {
		respondError(c, http.StatusUnauthorized, "totp_required", "a two-factor code is required")
		return
	}

	valid, err := accounts.VerifySecondFactor(user, input.Code)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !valid {
		respondError(c, http.StatusUnauthorized, "invalid_code", "invalid two-factor code")
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
