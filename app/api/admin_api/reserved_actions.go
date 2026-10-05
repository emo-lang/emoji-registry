package admin_api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/views/admin"
)

// Routes registers the admin pages under /admin (web session + admin only).
func Routes(r *gin.Engine) {
	group := r.Group("/admin", middlewares.RequireAdmin())
	{
		group.GET("/reserved", ReservedAction)
		group.POST("/reserved", AddReservedAction)
		group.POST("/reserved/:id/delete", DeleteReservedAction)
	}
}

// ReservedAction lists the managed reserved names plus the read-only
// built-in list.
func ReservedAction(c *gin.Context) {
	renderReserved(c, "", http.StatusOK)
}

// AddReservedAction reserves a name. Re-reserving an existing name updates
// its reason (idempotent).
func AddReservedAction(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	reason := strings.TrimSpace(c.PostForm("reason"))

	if !emoji.ValidNamePart(name) {
		renderReserved(c, "Name must be 1-64 characters of lowercase letters, digits, _ or -.", http.StatusUnprocessableEntity)
		return
	}

	existing, err := repo.FindOneBy[models.ReservedName](sql.H{"name": name})
	if err != nil {
		render.Error(c, err)
		return
	}

	if existing != nil {
		if err := repo.UpdateByID[models.ReservedName](existing.ID, sql.H{"reason": reason}); err != nil {
			render.Error(c, err)
			return
		}
	} else {
		if _, err := repo.CreateFrom[models.ReservedName](sql.H{"name": name, "reason": reason}); err != nil {
			render.Error(c, err)
			return
		}
	}

	c.Redirect(http.StatusFound, "/admin/reserved")
}

// DeleteReservedAction removes a managed reserved name.
func DeleteReservedAction(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err == nil {
		_ = repo.DeleteByID[models.ReservedName](sql.IdType(id))
	}

	c.Redirect(http.StatusFound, "/admin/reserved")
}

func renderReserved(c *gin.Context, errMsg string, status int) {
	user := middlewares.CurrentUser(c)

	rows, err := repo.FindAll[models.ReservedName]()
	if err != nil {
		render.Error(c, err)
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	render.HTMLStatus(c, status, admin.Reserved(user.Username, user.Admin, rows, emoji.BuiltinReservedNames(), errMsg))
}
