package accounts_api

import (
	"github.com/gin-gonic/gin"
)

// Routes registers the account and API-token endpoints under /api/v1.
func Routes(r *gin.RouterGroup) {
	r.POST("/signup", SignupAction)
	r.POST("/login", LoginAction)

	r.POST("/tokens", accountAuth(), CreateTokenAction)
	r.GET("/tokens", accountAuth(), ListTokensAction)
	r.DELETE("/tokens/:id", accountAuth(), DeleteTokenAction)
}
