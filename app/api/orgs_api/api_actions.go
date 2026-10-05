package orgs_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
)

// CreateOrgAction handles POST /api/v1/orgs.
func CreateOrgAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	var input struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with name and optional display_name")
		return
	}

	name := strings.TrimSpace(input.Name)
	displayName := strings.TrimSpace(input.DisplayName)

	if !emoji.ValidNamePart(name) {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "name must be 1-64 chars of [a-z0-9_-]")
		return
	}
	if reason, err := orgs.NameTakenReason(name); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	} else if reason != "" {
		code := "name_taken"
		if emoji.IsReserved(name) {
			code = "name_reserved"
		}
		respondError(c, http.StatusConflict, code, reason)
		return
	}

	org, err := orgs.Create(user, name, displayName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"name":         org.Name,
		"display_name": org.DisplayName,
	})
}

// ShowOrgAction handles GET /api/v1/orgs/:name.
func ShowOrgAction(c *gin.Context) {
	org, members, ok := loadOrgWithMembers(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, orgObject(org, members))
}

// AddMemberAction handles POST /api/v1/orgs/:name/members (org owners only).
func AddMemberAction(c *gin.Context) {
	caller := middlewares.CurrentUser(c)

	var input struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_params", "expected a JSON object with username and role")
		return
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = models.RoleMember
	}
	if role != models.RoleOwner && role != models.RoleMember {
		respondError(c, http.StatusUnprocessableEntity, "invalid_params", "role must be owner or member")
		return
	}

	org, _, ok := loadOrgWithMembers(c)
	if !ok {
		return
	}
	if !orgs.IsOwner(org.ID, caller.ID) {
		respondError(c, http.StatusForbidden, "forbidden", "only organization owners can manage members")
		return
	}

	target, err := repo.FindOneBy[models.User](sql.H{"username": strings.TrimSpace(input.Username)})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if target == nil {
		respondError(c, http.StatusNotFound, "user_not_found", "no such user "+input.Username)
		return
	}

	if m, err := orgs.MembershipOf(org.ID, target.ID); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	} else if m != nil {
		respondError(c, http.StatusConflict, "already_member", target.Username+" is already a member")
		return
	}

	if _, err := repo.CreateFrom[models.Membership](sql.H{
		"organization_id": org.ID,
		"user_id":         target.ID,
		"role":            role,
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	_, members, _ := loadOrgWithMembers(c)
	c.JSON(http.StatusCreated, orgObject(org, members))
}

// RemoveMemberAction handles DELETE /api/v1/orgs/:name/members/:username
// (org owners only). The last owner cannot be removed.
func RemoveMemberAction(c *gin.Context) {
	caller := middlewares.CurrentUser(c)

	org, _, ok := loadOrgWithMembers(c)
	if !ok {
		return
	}
	if !orgs.IsOwner(org.ID, caller.ID) {
		respondError(c, http.StatusForbidden, "forbidden", "only organization owners can manage members")
		return
	}

	target, err := repo.FindOneBy[models.User](sql.H{"username": c.Param("username")})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var membership *models.Membership
	if target != nil {
		membership, err = orgs.MembershipOf(org.ID, target.ID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	if membership == nil {
		respondError(c, http.StatusNotFound, "member_not_found", c.Param("username")+" is not a member")
		return
	}

	if membership.Role == models.RoleOwner {
		owners, err := repo.CountWhere[models.Membership](sql.H{"organization_id": org.ID, "role": models.RoleOwner})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if owners <= 1 {
			respondError(c, http.StatusConflict, "last_owner", "the last organization owner cannot be removed")
			return
		}
	}

	if err := repo.DeleteByID[models.Membership](membership.ID); err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	_, members, _ := loadOrgWithMembers(c)
	c.JSON(http.StatusOK, orgObject(org, members))
}

// loadOrgWithMembers resolves :name and renders org_not_found when needed.
func loadOrgWithMembers(c *gin.Context) (*models.Organization, []orgs.MemberView, bool) {
	org, err := orgs.FindByName(c.Param("name"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return nil, nil, false
	}
	if org == nil {
		respondError(c, http.StatusNotFound, "org_not_found", "no such organization "+c.Param("name"))
		return nil, nil, false
	}

	members, err := orgs.MemberViews(org.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return nil, nil, false
	}

	return org, members, true
}
