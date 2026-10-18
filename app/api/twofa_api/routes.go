package twofa_api

import (
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// WebRoutes registers the two-factor enrollment, challenge and account
// security pages.
func WebRoutes(r *gin.Engine) {
	r.GET("/account/2fa/setup", middlewares.RequirePendingTwoFactor(), ShowSetupAction)
	r.POST("/account/2fa/setup", middlewares.LimitAuthPage(), middlewares.RequirePendingTwoFactor(), SetupAction)

	r.GET("/login/2fa", middlewares.RequirePendingTwoFactor(), ShowChallengeAction)
	r.POST("/login/2fa", middlewares.LimitAuthPage(), middlewares.RequirePendingTwoFactor(), ChallengeAction)

	account := r.Group("/account", middlewares.RequireWebAuth())
	{
		account.GET("/2fa", SecurityAction)
		account.POST("/2fa/recovery-codes", RegenerateRecoveryCodesAction)
	}
}
