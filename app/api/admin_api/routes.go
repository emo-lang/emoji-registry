package admin_api

import (
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
)

// Routes registers the admin area under /admin. The env-credentials login
// form is public (it 404s unless ADMIN_USERNAME/ADMIN_PASSWORD are set);
// everything else demands an admin via RequireAdmin.
func Routes(r *gin.Engine) {
	r.GET("/admin/login", ShowAdminLoginAction)
	r.POST("/admin/login", middlewares.LimitAuthPage(), AdminLoginAction)

	group := r.Group("/admin", middlewares.RequireAdmin())
	{
		group.GET("", PackagesAction)
		group.POST("/logout", AdminLogoutAction)
		group.GET("/reserved", ReservedAction)
		group.POST("/reserved", AddReservedAction)
		group.POST("/reserved/:id/delete", DeleteReservedAction)
	}
}

// adminViewer returns the identity the shared top navigation should show:
// the signed-in site account (empty when the admin came in through
// /admin/login) and whether the Admin link applies.
func adminViewer(c *gin.Context) (string, bool) {
	if user := middlewares.CurrentUser(c); user != nil {
		return user.Username, user.Admin || middlewares.CurrentAdmin(c)
	}
	return "", middlewares.CurrentAdmin(c)
}

// versionCounts maps package IDs to their version totals for the packages
// list — one query instead of one per row.
func versionCounts() (map[sql.IdType]int, error) {
	versions, err := repo.FindAll[models.Version]()
	if err != nil {
		return nil, err
	}

	counts := make(map[sql.IdType]int, len(versions))
	for _, v := range versions {
		counts[v.PackageID]++
	}
	return counts, nil
}
