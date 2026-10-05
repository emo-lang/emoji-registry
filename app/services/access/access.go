// Package access decides who may read a package. Public packages are
// readable by everyone; private packages by the owner account, any member of
// the owning organization, and read-scoped API tokens of those users.
package access

import (
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// CanRead reports whether user (nil for anonymous) may read pkg.
func CanRead(user *models.User, pkg *models.Package) bool {
	if !pkg.Private() {
		return true
	}
	if user == nil {
		return false
	}
	if pkg.UserID == user.ID {
		return true
	}

	org, err := orgs.FindByName(pkg.OwnerScope)
	if err != nil || org == nil {
		return false
	}

	m, err := orgs.MembershipOf(org.ID, user.ID)
	return err == nil && m != nil
}

// TokenCanRead reports whether an API token may read pkg: the token needs the
// read scope, and its owner needs read access to the package.
func TokenCanRead(token *models.APIToken, user *models.User, pkg *models.Package) bool {
	if token == nil || user == nil {
		return false
	}
	if !hasScope(token, "read") {
		return false
	}
	return CanRead(user, pkg)
}

func hasScope(token *models.APIToken, scope string) bool {
	for _, s := range splitScopes(token.Scopes) {
		if s == scope {
			return true
		}
	}
	return false
}

func splitScopes(scopes string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(scopes); i++ {
		if i == len(scopes) || scopes[i] == ',' {
			out = append(out, scopes[start:i])
			start = i + 1
		}
	}
	return out
}
