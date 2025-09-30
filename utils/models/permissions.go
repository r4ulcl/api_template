package models

// Permissions defines which resources a role can access for each HTTP verb and scope.
type Permissions struct {
	Get       []string
	GetOwn    []string
	Post      []string
	PostOwn   []string
	Put       []string
	PutOwn    []string
	Patch     []string
	PatchOwn  []string
	Delete    []string
	DeleteOwn []string
}

/* -------------------------------------------------------------------------- */
/*                            ANONYMOUS PERMISSIONS                           */
/* -------------------------------------------------------------------------- */

var AnonymousGetResources = []string{}
var AnonymousGetResourcesOwn = []string{}

var AnonymousPostResources = []string{}
var AnonymousPostResourcesOwn = []string{}

var AnonymousPutResources = []string{}
var AnonymousPutResourcesOwn = []string{}

var AnonymousPatchResources = []string{}
var AnonymousPatchResourcesOwn = []string{}

var AnonymousDeleteResources = []string{}
var AnonymousDeleteResourcesOwn = []string{}

/* -------------------------------------------------------------------------- */
/*                              USER PERMISSIONS                              */
/* -------------------------------------------------------------------------- */

var UserGetResources = []string{"example2"}
var UserGetResourcesOwn = []string{"example1", "exampleRelational"}

var UserPostResources = []string{"example2"}
var UserPostResourcesOwn = []string{"example1", "exampleRelational"}

var UserPutResources = []string{}
var UserPutResourcesOwn = []string{"example1", "example2", "exampleRelational"}

var UserPatchResources = []string{}
var UserPatchResourcesOwn = []string{"example1", "example2", "exampleRelational"}

var UserDeleteResources = []string{}
var UserDeleteResourcesOwn = []string{"example1", "example2", "exampleRelational"}

/* -------------------------------------------------------------------------- */
/*                             REVIEWER PERMISSIONS                           */
/* -------------------------------------------------------------------------- */

// Reviewer: read-only access to all models except user data.
var ReviewerGetResources = []string{"example1", "example2", "exampleRelational"}
var ReviewerGetResourcesOwn = []string{}

var ReviewerPostResources = []string{}
var ReviewerPostResourcesOwn = []string{}

var ReviewerPutResources = []string{}
var ReviewerPutResourcesOwn = []string{}

var ReviewerPatchResources = []string{}
var ReviewerPatchResourcesOwn = []string{}

var ReviewerDeleteResources = []string{}
var ReviewerDeleteResourcesOwn = []string{}

/* -------------------------------------------------------------------------- */
/*                              ADMIN PERMISSIONS                             */
/* -------------------------------------------------------------------------- */

var AdminGetResources = []string{"auditLog", "user", "example1", "example2", "exampleRelational"}
var AdminGetResourcesOwn = []string{}

var AdminPostResources = []string{"auditLog", "user", "example1", "example2", "exampleRelational"}
var AdminPostResourcesOwn = []string{}

var AdminPutResources = []string{"auditLog", "user", "example1", "example2", "exampleRelational"}
var AdminPutResourcesOwn = []string{}

var AdminPatchResources = []string{"auditLog", "user", "example1", "example2", "exampleRelational"}
var AdminPatchResourcesOwn = []string{}

var AdminDeleteResources = []string{"auditLog", "user", "example1", "example2", "exampleRelational"}
var AdminDeleteResourcesOwn = []string{}

/* -------------------------------------------------------------------------- */
/*                              ROLE → PERMISSIONS                            */
/* -------------------------------------------------------------------------- */

var RolePermissions = map[string]Permissions{
	"anonymous": {
		Get:       AnonymousGetResources,
		GetOwn:    AnonymousGetResourcesOwn,
		Post:      AnonymousPostResources,
		PostOwn:   AnonymousPostResourcesOwn,
		Put:       AnonymousPutResources,
		PutOwn:    AnonymousPutResourcesOwn,
		Patch:     AnonymousPatchResources,
		PatchOwn:  AnonymousPatchResourcesOwn,
		Delete:    AnonymousDeleteResources,
		DeleteOwn: AnonymousDeleteResourcesOwn,
	},
	"user": {
		Get:       UserGetResources,
		GetOwn:    UserGetResourcesOwn,
		Post:      UserPostResources,
		PostOwn:   UserPostResourcesOwn,
		Put:       UserPutResources,
		PutOwn:    UserPutResourcesOwn,
		Patch:     UserPatchResources,
		PatchOwn:  UserPatchResourcesOwn,
		Delete:    UserDeleteResources,
		DeleteOwn: UserDeleteResourcesOwn,
	},
	"reviewer": {
		Get:       ReviewerGetResources,
		GetOwn:    ReviewerGetResourcesOwn,
		Post:      ReviewerPostResources,
		PostOwn:   ReviewerPostResourcesOwn,
		Put:       ReviewerPutResources,
		PutOwn:    ReviewerPutResourcesOwn,
		Patch:     ReviewerPatchResources,
		PatchOwn:  ReviewerPatchResourcesOwn,
		Delete:    ReviewerDeleteResources,
		DeleteOwn: ReviewerDeleteResourcesOwn,
	},
	"admin": {
		Get:       AdminGetResources,
		GetOwn:    AdminGetResourcesOwn,
		Post:      AdminPostResources,
		PostOwn:   AdminPostResourcesOwn,
		Put:       AdminPutResources,
		PutOwn:    AdminPutResourcesOwn,
		Patch:     AdminPatchResources,
		PatchOwn:  AdminPatchResourcesOwn,
		Delete:    AdminDeleteResources,
		DeleteOwn: AdminDeleteResourcesOwn,
	},
}
