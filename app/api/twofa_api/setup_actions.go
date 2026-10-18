package twofa_api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"
	"rsc.io/qr"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
	"github.com/emo-lang/emoji-registry/app/services/totp"
	"github.com/emo-lang/emoji-registry/app/views/twofa"
)

const issuer = "Emo Registry"

// ShowSetupAction renders the enrollment page for a pending session.
func ShowSetupAction(c *gin.Context) {
	user := middlewares.PendingTwoFactorUser(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	if user.TOTPEnabled {
		c.Redirect(http.StatusFound, "/login/2fa")
		return
	}

	renderSetup(c, user, "", http.StatusOK)
}

// SetupAction confirms a code against the generated secret, enables the
// second factor, stores recovery codes and promotes the session.
func SetupAction(c *gin.Context) {
	user := middlewares.PendingTwoFactorUser(c)
	session := middlewares.PendingTwoFactorSession(c)
	if user == nil || session == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	if user.TOTPEnabled {
		c.Redirect(http.StatusFound, "/login/2fa")
		return
	}

	code := strings.TrimSpace(c.PostForm("code"))
	if !totp.Validate(user.TOTPSecret, code, time.Now()) {
		renderSetup(c, user, "Invalid code. Make sure your device's clock is correct and try again.", http.StatusUnprocessableEntity)
		return
	}

	codes, err := accounts.GenerateRecoveryCodes()
	if err != nil {
		render.Error(c, err)
		return
	}

	if err := repo.UpdateByID[models.User](user.ID, sql.H{"totp_enabled": true}); err != nil {
		render.Error(c, err)
		return
	}
	user.TOTPEnabled = true

	if err := accounts.StoreRecoveryCodes(user.ID, codes); err != nil {
		render.Error(c, err)
		return
	}

	if err := middlewares.CompleteTwoFactor(c, session, user); err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, twofa.Recovery(user.Username, user.Admin, strings.Join(codes, "\n")))
}

func renderSetup(c *gin.Context, user *models.User, errMsg string, status int) {
	secret, err := accounts.EnsureTOTPSecret(user)
	if err != nil {
		render.Error(c, err)
		return
	}

	code, err := qr.Encode(totp.ProvisioningURI(issuer, user.Email, secret), qr.M)
	if err != nil {
		render.Error(c, err)
		return
	}
	qrDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())

	render.HTMLStatus(c, status, twofa.Setup(user.Username, qrDataURI, secret, errMsg))
}
