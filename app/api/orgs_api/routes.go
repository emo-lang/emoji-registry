package orgs_api

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// APIRoutes registers the organization JSON endpoints under /api/v1.
func APIRoutes(r *gin.RouterGroup) {
	r.POST("/orgs", middlewares.AccountAuth(), CreateOrgAction)
	r.GET("/orgs/:name", ShowOrgAction)
	r.POST("/orgs/:name/members", middlewares.AccountAuth(), AddMemberAction)
	r.DELETE("/orgs/:name/members/:username", middlewares.AccountAuth(), RemoveMemberAction)
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func orgObject(org *models.Organization, members []orgs.MemberView) gin.H {
	list := make([]gin.H, 0, len(members))
	for _, m := range members {
		list = append(list, gin.H{"username": m.Username, "role": m.Role})
	}

	return gin.H{
		"name":         org.Name,
		"display_name": org.DisplayName,
		"members":      list,
		"created_at":   org.CreatedAt.UTC().Format(time.RFC3339),
	}
}
