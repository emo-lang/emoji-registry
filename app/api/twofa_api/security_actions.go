package twofa_api

import (
	"net/http"
	"strings"
	"time"

	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/services/accounts"
	"github.com/emo-lang/emoji-registry/app/services/totp"
	"github.com/emo-lang/emoji-registry/app/views/twofa"
)

// SecurityAction renders the account security page.
func SecurityAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)
	remaining, err := accounts.CountUnusedRecoveryCodes(user.ID)
	if err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, twofa.Security(user.Username, user.Admin, user.TOTPEnabled, int(remaining), "", ""))
}

// RegenerateRecoveryCodesAction replaces the user's recovery codes after a
// fresh TOTP challenge.
func RegenerateRecoveryCodesAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)
	code := strings.TrimSpace(c.PostForm("code"))

	if !totp.Validate(user.TOTPSecret, code, time.Now()) {
		remaining, err := accounts.CountUnusedRecoveryCodes(user.ID)
		if err != nil {
			render.Error(c, err)
			return
		}
		render.HTMLStatus(c, http.StatusUnprocessableEntity,
			twofa.Security(user.Username, user.Admin, user.TOTPEnabled, int(remaining), "", "Invalid code."))
		return
	}

	codes, err := accounts.GenerateRecoveryCodes()
	if err != nil {
		render.Error(c, err)
		return
	}
	if err := accounts.StoreRecoveryCodes(user.ID, codes); err != nil {
		render.Error(c, err)
		return
	}

	render.HTML(c, twofa.Security(user.Username, user.Admin, true, len(codes), strings.Join(codes, "\n"), ""))
}
