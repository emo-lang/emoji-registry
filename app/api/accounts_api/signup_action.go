package accounts_api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/services/accounts"
)

// SignupAction handles POST /api/v1/signup. It creates the account but does
// not log the user in.
func SignupAction(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with username, email and password")
		return
	}

	user, err := accounts.Signup(input.Username, input.Email, input.Password)
	if err != nil {
		var signupErr *accounts.SignupError
		if errors.As(err, &signupErr) {
			respondError(c, signupErrorStatus(signupErr.Code), signupErr.Code, signupErr.Message)
			return
		}
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"username": user.Username,
		"email":    user.Email,
	})
}

func signupErrorStatus(code string) int {
	switch code {
	case "username_taken", "email_taken":
		return http.StatusConflict
	case "name_reserved":
		return http.StatusForbidden
	default:
		return http.StatusUnprocessableEntity
	}
}
