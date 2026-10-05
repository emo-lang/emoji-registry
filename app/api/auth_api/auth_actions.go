package auth_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/views/auth"
)

// ShowSignupAction renders the registration form.
func ShowSignupAction(c *gin.Context) {
	render.HTML(c, auth.Signup("", "", ""))
}

// SignupAction processes the registration form: validates, creates the user
// and signs them in with a fresh session cookie.
func SignupAction(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	fail := func(message string) {
		render.HTMLStatus(c, http.StatusUnprocessableEntity, auth.Signup(message, username, email))
	}

	if !emoji.ValidNamePart(username) {
		fail("Username must be 1-64 characters of lowercase letters, digits, _ or -.")
		return
	}
	if emoji.IsReserved(username) {
		fail("Username " + username + " is reserved for the official stdlib.")
		return
	}
	if !strings.Contains(email, "@") || len(email) > 255 {
		fail("A valid email address is required.")
		return
	}
	if len(password) < 8 {
		fail("Password must be at least 8 characters.")
		return
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"username": username}); err != nil {
		render.Error(c, err)
		return
	} else if exists {
		fail("Username " + username + " is taken.")
		return
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"email": email}); err != nil {
		render.Error(c, err)
		return
	} else if exists {
		fail("Email " + email + " is already registered.")
		return
	}

	digest, err := utils.EncryptPassword(password)
	if err != nil {
		render.Error(c, err)
		return
	}

	user, err := repo.CreateFrom[models.User](sql.H{
		"username":        username,
		"email":           email,
		"password_digest": digest,
	})
	if err != nil {
		render.Error(c, err)
		return
	}

	if !signIn(c, user) {
		render.ErrorMessage(c, "failed to create session")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// ShowLoginAction renders the login form.
func ShowLoginAction(c *gin.Context) {
	render.HTML(c, auth.Login("", ""))
}

// LoginAction processes the login form.
func LoginAction(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	fail := func(message string) {
		render.HTMLStatus(c, http.StatusUnprocessableEntity, auth.Login(message, email))
	}

	user, err := repo.FindOneBy[models.User](sql.H{"email": email})
	if err != nil {
		render.Error(c, err)
		return
	}
	if user == nil || !utils.ComparePassword(utils.PasswordDigest(user.PasswordDigest), password) {
		fail("Invalid email or password.")
		return
	}

	if !signIn(c, user) {
		render.ErrorMessage(c, "failed to create session")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// LogoutAction destroys the current session and clears the cookie.
func LogoutAction(c *gin.Context) {
	if cookie, err := c.Cookie(middlewares.SessionCookie); err == nil && cookie != "" {
		_ = repo.DeleteWhere[models.Session](sql.H{"token": cookie})
	}

	c.SetCookie(middlewares.SessionCookie, "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

func signIn(c *gin.Context, user *models.User) bool {
	session, err := middlewares.CreateSession(user)
	if err != nil {
		return false
	}

	c.SetCookie(middlewares.SessionCookie, session.Token, int(middlewares.SessionTTL.Seconds()), "/", "", false, true)
	return true
}
