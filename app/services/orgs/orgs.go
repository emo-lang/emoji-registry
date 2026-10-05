// Package orgs holds the organization and package-scope permission rules:
// who may publish under a scope, and whether a name is free to register as a
// username or an organization.
package orgs

import (
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// FindByName looks up an organization by name; nil when missing.
func FindByName(name string) (*models.Organization, error) {
	return repo.FindOneBy[models.Organization](sql.H{"name": name})
}

// MembershipOf returns the membership of userID in orgID; nil when the user
// is not a member.
func MembershipOf(orgID, userID sql.IdType) (*models.Membership, error) {
	return repo.FindOneBy[models.Membership](sql.H{"organization_id": orgID, "user_id": userID})
}

// IsOwner reports whether userID is an owner of the organization.
func IsOwner(orgID, userID sql.IdType) bool {
	m, err := MembershipOf(orgID, userID)
	return err == nil && m != nil && m.Role == models.RoleOwner
}

// Members lists an organization's memberships, owners first.
func Members(orgID sql.IdType) ([]*models.Membership, error) {
	members, err := repo.FindBy[models.Membership](sql.H{"organization_id": orgID})
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			if members[j].Role == models.RoleOwner && members[i].Role != models.RoleOwner {
				members[i], members[j] = members[j], members[i]
			}
		}
	}

	return members, nil
}

// CanPublishAs reports whether user may publish, yank or edit metadata under
// the given package scope: their own username, or any organization they
// belong to (owners and members alike).
func CanPublishAs(user *models.User, scope string) bool {
	if user.Username == scope {
		return true
	}

	org, err := FindByName(scope)
	if err != nil || org == nil {
		return false
	}

	m, err := MembershipOf(org.ID, user.ID)
	return err == nil && m != nil
}

// Create creates an organization and makes user its first owner.
func Create(user *models.User, name, displayName string) (*models.Organization, error) {
	org, err := repo.CreateFrom[models.Organization](sql.H{
		"name":         name,
		"display_name": displayName,
		"user_id":      user.ID,
	})
	if err != nil {
		return nil, err
	}

	if _, err := repo.CreateFrom[models.Membership](sql.H{
		"organization_id": org.ID,
		"user_id":         user.ID,
		"role":            models.RoleOwner,
	}); err != nil {
		return nil, err
	}

	return org, nil
}

// MemberView pairs a membership with the member's username.
type MemberView struct {
	Username string
	Role     string
}

// MemberViews lists an organization's members (owners first) with usernames
// resolved.
func MemberViews(orgID sql.IdType) ([]MemberView, error) {
	memberships, err := Members(orgID)
	if err != nil {
		return nil, err
	}

	members := make([]MemberView, 0, len(memberships))
	for _, m := range memberships {
		user, err := repo.FindByID[models.User](m.UserID)
		if err != nil || user == nil {
			continue
		}
		members = append(members, MemberView{Username: user.Username, Role: m.Role})
	}

	return members, nil
}

// NameTakenReason explains why a username/organization name cannot be
// registered, or returns "" when the name is free. Users and organizations
// share one namespace because both act as package scopes.
func NameTakenReason(name string) (string, error) {
	if emoji.IsReserved(name) {
		return name + " is reserved for the official stdlib", nil
	}

	if exists, err := repo.ExistsWhere[models.User](sql.H{"username": name}); err != nil {
		return "", err
	} else if exists {
		return name + " is taken by a user account", nil
	}

	if exists, err := repo.ExistsWhere[models.Organization](sql.H{"name": name}); err != nil {
		return "", err
	} else if exists {
		return name + " is taken by an organization", nil
	}

	return "", nil
}
