package search_api

import (
	"strconv"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/views/search"
)

// PerPage is the search page size.
const PerPage = 20

// SearchAction handles GET /search?q=&page=. Matching is a case-insensitive
// substring match over owner scope, name and description, portable across
// SQLite/Postgres/MySQL via LOWER(col) LIKE.
func SearchAction(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page < 1 {
		page = 1
	}

	var results []*models.Package
	var total int64

	if q != "" {
		needle := "%" + strings.ToLower(q) + "%"
		cond := sql.AnyOf(
			sql.Like("LOWER(owner_scope)", needle),
			sql.Like("LOWER(name)", needle),
			sql.Like("LOWER(description)", needle),
		)

		total, err = repo.Count(repo.CurrentDB(),
			sql.Select("COUNT(*)").FromTable(sql.TableFor(models.Package{})).Where(cond))
		if err != nil {
			render.Error(c, err)
			return
		}

		results, err = repo.Find[models.Package](repo.CurrentDB(),
			sql.All(models.Package{}).Where(cond).OrderBy("downloads DESC").Page(page, PerPage))
		if err != nil {
			render.Error(c, err)
			return
		}
	}

	username := ""
	if user := middlewares.CurrentUser(c); user != nil {
		username = user.Username
	}

	render.HTML(c, search.Results(username, q, results, page, PerPage, total))
}
