package twofa_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
	"github.com/emo-lang/emoji-registry/app/views/twofa"
)

// ShowChallengeAction asks a pending session for its second factor.
func ShowChallengeAction(c *gin.Context) {
	user := middlewares.PendingTwoFactorUser(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	if !user.TOTPEnabled {
		c.Redirect(http.StatusFound, "/account/2fa/setup")
		return
	}

	render.HTML(c, twofa.Challenge(user.Username, ""))
}

// ChallengeAction verifies a TOTP or recovery code and promotes the session.
func ChallengeAction(c *gin.Context) {
	user := middlewares.PendingTwoFactorUser(c)
	session := middlewares.PendingTwoFactorSession(c)
	if user == nil || session == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	if !user.TOTPEnabled {
		c.Redirect(http.StatusFound, "/account/2fa/setup")
		return
	}

	valid, err := accounts.VerifySecondFactor(user, strings.TrimSpace(c.PostForm("code")))
	if err != nil {
		render.Error(c, err)
		return
	}
	if !valid {
		render.HTMLStatus(c, http.StatusUnprocessableEntity, twofa.Challenge(user.Username, "Invalid code."))
		return
	}

	if err := middlewares.CompleteTwoFactor(c, session, user); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/")
}
