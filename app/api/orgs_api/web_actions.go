package orgs_api

import (
	"net/http"
	"strings"

	"github.com/daqing/airway/lib/render"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/gin-gonic/gin"

	"github.com/emo-lang/emoji-registry/app/middlewares"
	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	"github.com/emo-lang/emoji-registry/app/services/orgs"
	"github.com/emo-lang/emoji-registry/app/views/errors"
	orgviews "github.com/emo-lang/emoji-registry/app/views/orgs"
)

// WebRoutes registers the organization pages at the root.
func WebRoutes(r *gin.Engine) {
	r.GET("/orgs", middlewares.RequireWebAuth(), IndexAction)
	r.GET("/orgs/new", middlewares.RequireWebAuth(), NewOrgAction)
	r.POST("/orgs", middlewares.RequireWebAuth(), CreateOrgWebAction)
	r.GET("/orgs/:name", ShowOrgPageAction)
	r.POST("/orgs/:name/members", middlewares.RequireWebAuth(), AddMemberWebAction)
	r.POST("/orgs/:name/members/:username/delete", middlewares.RequireWebAuth(), RemoveMemberWebAction)
	r.POST("/orgs/:name/members/:username/role", middlewares.RequireWebAuth(), ChangeRoleWebAction)
}

// IndexAction lists the organizations the current user belongs to.
func IndexAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	memberships, err := repo.FindBy[models.Membership](sql.H{"user_id": user.ID})
	if err != nil {
		render.Error(c, err)
		return
	}

	organizations := []*models.Organization{}
	for _, m := range memberships {
		if org, err := repo.FindByID[models.Organization](m.OrganizationID); err == nil && org != nil {
			organizations = append(organizations, org)
		}
	}

	render.HTML(c, orgviews.Index(user.Username, organizations, ""))
}

// NewOrgAction renders the create form.
func NewOrgAction(c *gin.Context) {
	render.HTML(c, orgviews.New(middlewares.CurrentUser(c).Username, "", "", ""))
}

// CreateOrgWebAction processes the create form.
func CreateOrgWebAction(c *gin.Context) {
	user := middlewares.CurrentUser(c)

	name := strings.TrimSpace(c.PostForm("name"))
	displayName := strings.TrimSpace(c.PostForm("display_name"))

	fail := func(message string) {
		render.HTMLStatus(c, http.StatusUnprocessableEntity, orgviews.New(user.Username, message, name, displayName))
	}

	if !emoji.ValidNamePart(name) {
		fail("Name must be 1-64 characters of lowercase letters, digits, _ or -.")
		return
	}
	if reason, err := orgs.NameTakenReason(name); err != nil {
		render.Error(c, err)
		return
	} else if reason != "" {
		fail(reason + ".")
		return
	}

	org, err := orgs.Create(user, name, displayName)
	if err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/orgs/"+org.Name)
}

// ShowOrgPageAction renders the organization page.
func ShowOrgPageAction(c *gin.Context) {
	username := ""
	if user := middlewares.CurrentUser(c); user != nil {
		username = user.Username
	}

	org, err := orgs.FindByName(c.Param("name"))
	if err != nil {
		render.Error(c, err)
		return
	}
	if org == nil {
		render.HTMLStatus(c, http.StatusNotFound, errors.NotFound(username, "Organization "+c.Param("name")+" does not exist."))
		return
	}

	members, err := orgs.MemberViews(org.ID)
	if err != nil {
		render.Error(c, err)
		return
	}

	pkgs, err := repo.FindBy[models.Package](sql.H{"owner_scope": org.Name})
	if err != nil {
		render.Error(c, err)
		return
	}

	isOwner := false
	if user := middlewares.CurrentUser(c); user != nil {
		isOwner = orgs.IsOwner(org.ID, user.ID)
	}

	render.HTML(c, orgviews.Show(username, org, members, pkgs, isOwner, ""))
}

// AddMemberWebAction processes the add-member form (org owners only).
func AddMemberWebAction(c *gin.Context) {
	caller := middlewares.CurrentUser(c)

	org, err := orgs.FindByName(c.Param("name"))
	if err != nil {
		render.Error(c, err)
		return
	}
	if org == nil || !orgs.IsOwner(org.ID, caller.ID) {
		render.HTMLStatus(c, http.StatusNotFound, errors.NotFound(caller.Username, "Organization "+c.Param("name")+" does not exist."))
		return
	}

	fail := func(message string) {
		members, _ := orgs.MemberViews(org.ID)
		pkgs, _ := repo.FindBy[models.Package](sql.H{"owner_scope": org.Name})
		render.HTMLStatus(c, http.StatusUnprocessableEntity, orgviews.Show(caller.Username, org, members, pkgs, true, message))
	}

	role := strings.TrimSpace(c.PostForm("role"))
	if role == "" {
		role = models.RoleMember
	}
	if role != models.RoleOwner && role != models.RoleMember {
		fail("Role must be owner or member.")
		return
	}

	target, err := repo.FindOneBy[models.User](sql.H{"username": strings.TrimSpace(c.PostForm("username"))})
	if err != nil {
		render.Error(c, err)
		return
	}
	if target == nil {
		fail("No such user.")
		return
	}

	if m, err := orgs.MembershipOf(org.ID, target.ID); err != nil {
		render.Error(c, err)
		return
	} else if m != nil {
		fail(target.Username + " is already a member.")
		return
	}

	if _, err := repo.CreateFrom[models.Membership](sql.H{
		"organization_id": org.ID,
		"user_id":         target.ID,
		"role":            role,
	}); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/orgs/"+org.Name)
}

// RemoveMemberWebAction removes a member (org owners only, last owner
// protected).
func RemoveMemberWebAction(c *gin.Context) {
	caller := middlewares.CurrentUser(c)

	org, membership, ok := loadMemberForEdit(c, caller)
	if !ok {
		return
	}

	if membership.Role == models.RoleOwner {
		owners, err := repo.CountWhere[models.Membership](sql.H{"organization_id": org.ID, "role": models.RoleOwner})
		if err != nil {
			render.Error(c, err)
			return
		}
		if owners <= 1 {
			failShow(c, org, "The last organization owner cannot be removed.")
			return
		}
	}

	if err := repo.DeleteByID[models.Membership](membership.ID); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/orgs/"+org.Name)
}

// ChangeRoleWebAction flips a member between owner and member (org owners
// only; the last owner cannot be demoted).
func ChangeRoleWebAction(c *gin.Context) {
	caller := middlewares.CurrentUser(c)

	org, membership, ok := loadMemberForEdit(c, caller)
	if !ok {
		return
	}

	role := strings.TrimSpace(c.PostForm("role"))
	if role != models.RoleOwner && role != models.RoleMember {
		failShow(c, org, "Role must be owner or member.")
		return
	}

	if membership.Role == models.RoleOwner && role == models.RoleMember {
		owners, err := repo.CountWhere[models.Membership](sql.H{"organization_id": org.ID, "role": models.RoleOwner})
		if err != nil {
			render.Error(c, err)
			return
		}
		if owners <= 1 {
			failShow(c, org, "The last organization owner cannot be demoted.")
			return
		}
	}

	if err := repo.UpdateByID[models.Membership](membership.ID, sql.H{"role": role}); err != nil {
		render.Error(c, err)
		return
	}

	c.Redirect(http.StatusFound, "/orgs/"+org.Name)
}

// loadMemberForEdit resolves the org and the target membership, enforcing
// that the caller is an org owner.
func loadMemberForEdit(c *gin.Context, caller *models.User) (*models.Organization, *models.Membership, bool) {
	org, err := orgs.FindByName(c.Param("name"))
	if err != nil {
		render.Error(c, err)
		return nil, nil, false
	}
	if org == nil || !orgs.IsOwner(org.ID, caller.ID) {
		render.HTMLStatus(c, http.StatusNotFound, errors.NotFound(caller.Username, "Organization "+c.Param("name")+" does not exist."))
		return nil, nil, false
	}

	target, err := repo.FindOneBy[models.User](sql.H{"username": c.Param("username")})
	if err != nil {
		render.Error(c, err)
		return nil, nil, false
	}

	var membership *models.Membership
	if target != nil {
		membership, err = orgs.MembershipOf(org.ID, target.ID)
		if err != nil {
			render.Error(c, err)
			return nil, nil, false
		}
	}
	if membership == nil {
		failShow(c, org, "No such member.")
		return nil, nil, false
	}

	return org, membership, true
}

func failShow(c *gin.Context, org *models.Organization, message string) {
	user := middlewares.CurrentUser(c)
	members, _ := orgs.MemberViews(org.ID)
	pkgs, _ := repo.FindBy[models.Package](sql.H{"owner_scope": org.Name})
	render.HTMLStatus(c, http.StatusUnprocessableEntity, orgviews.Show(user.Username, org, members, pkgs, true, message))
}
