package accounts_api

import (
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
)

// Routes registers the account and API-token endpoints under /api/v1.
func Routes(r *gin.RouterGroup) {
	r.POST("/signup", middlewares.LimitAuthAPI(), SignupAction)
	r.POST("/login", middlewares.LimitAuthAPI(), LoginAction)

	r.POST("/tokens", middlewares.AccountAuth(), CreateTokenAction)
	r.GET("/tokens", middlewares.AccountAuth(), ListTokensAction)
	r.DELETE("/tokens/:id", middlewares.AccountAuth(), DeleteTokenAction)
}
