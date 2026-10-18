// Package accounts holds the account registration rules shared by the JSON
// API and the web signup form.
package accounts

import (
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/utils"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// SignupError reports why a registration was rejected. Code is stable and
// machine-readable; Message is meant for the user.
type SignupError struct {
	Code    string
	Message string
}

func (e *SignupError) Error() string {
	return e.Message
}

// NormalizeEmail trims and lowercases an address, so registrations that
// differ only in case collide instead of creating a second account.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Signup validates a registration and creates the account. Missing fields and
// name or email conflicts return a *SignupError; any other error is internal.
func Signup(username, email, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	email = NormalizeEmail(email)

	if username == "" {
		return nil, &SignupError{Code: "invalid_params", Message: "username is required"}
	}
	if !emoji.ValidNamePart(username) {
		return nil, &SignupError{Code: "invalid_params", Message: "username must be a lowercase identifier: [a-z][a-z0-9_]{0,63}"}
	}
	if conflict := nameConflict(username); conflict != nil {
		return nil, conflict
	}
	if email == "" {
		return nil, &SignupError{Code: "invalid_params", Message: "email is required"}
	}
	if !strings.Contains(email, "@") || len(email) > 255 {
		return nil, &SignupError{Code: "invalid_params", Message: "a valid email is required"}
	}
	if strings.TrimSpace(password) == "" {
		return nil, &SignupError{Code: "invalid_params", Message: "password is required"}
	}
	if len(password) < 8 {
		return nil, &SignupError{Code: "invalid_params", Message: "password must be at least 8 characters"}
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"email": email}); err != nil {
		return nil, err
	} else if exists {
		return nil, emailConflict(email)
	}

	digest, err := utils.EncryptPassword(password)
	if err != nil {
		return nil, err
	}

	user, err := repo.CreateFrom[models.User](sql.H{
		"username":        username,
		"email":           email,
		"password_digest": digest,
	})
	if err != nil {
		// The unique indexes are the final authority: a concurrent signup may
		// have claimed the name or address since the checks above.
		if conflict := nameConflict(username); conflict != nil {
			return nil, conflict
		}
		if exists, existsErr := repo.ExistsWhere[models.User](sql.H{"email": email}); existsErr == nil && exists {
			return nil, emailConflict(email)
		}
		return nil, err
	}

	return user, nil
}

// nameConflict reports a username/organization namespace clash, or nil when
// the name is free.
func nameConflict(username string) *SignupError {
	reason, err := orgs.NameTakenReason(username)
	if err != nil || reason == "" {
		return nil
	}

	code := "username_taken"
	if emoji.IsReserved(username) {
		code = "name_reserved"
	}
	return &SignupError{Code: code, Message: reason}
}

func emailConflict(email string) *SignupError {
	return &SignupError{Code: "email_taken", Message: "email " + email + " is already registered"}
}
