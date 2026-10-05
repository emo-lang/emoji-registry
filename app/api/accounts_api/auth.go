package accounts_api

import (
	"github.com/gin-gonic/gin"
)

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
