package admin_api

import (
	"net/http"

	"github.com/daqing/airway/lib/render"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/views/admin"
	"github.com/emo-lang/emoji-registry/app/views/errors"
)

// ShowAdminLoginAction renders the env-credentials login form. Without
// ADMIN_USERNAME/ADMIN_PASSWORD it answers 404 so the admin sign-in stays
// invisible, mirroring RequireAdmin.
func ShowAdminLoginAction(c *gin.Context) {
	if !middlewares.AdminEnvConfigured() {
		notFound(c)
		return
	}
	if middlewares.IsAdmin(c) {
		c.Redirect(http.StatusFound, "/admin")
		return
	}
	render.HTML(c, admin.Login("", ""))
}

// AdminLoginAction signs in the env-configured admin: on match it issues the
// signed admin cookie; any mismatch re-renders the form.
func AdminLoginAction(c *gin.Context) {
	if !middlewares.AdminEnvConfigured() {
		notFound(c)
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")

	if !middlewares.CheckAdminEnv(username, password) {
		render.HTMLStatus(c, http.StatusUnprocessableEntity, admin.Login("Invalid username or password.", username))
		return
	}

	middlewares.SignInAdmin(c)
	c.Redirect(http.StatusFound, "/admin")
}

// AdminLogoutAction clears the env-credentials admin cookie.
func AdminLogoutAction(c *gin.Context) {
	middlewares.SignOutAdmin(c)
	c.Redirect(http.StatusFound, "/admin/login")
}

func notFound(c *gin.Context) {
	render.HTMLStatus(c, http.StatusNotFound, errors.NotFound("", "Not found."))
}
