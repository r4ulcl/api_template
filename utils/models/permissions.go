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
var UserGetResourcesOwn = []string{"example1", "exampleRelational", "auditLog"}

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

var AdminPostResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPostResourcesOwn = []string{}

var AdminPutResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPutResourcesOwn = []string{}

var AdminPatchResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPatchResourcesOwn = []string{}

var AdminDeleteResources = []string{"user", "example1", "example2", "exampleRelational"}
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

/* -------------------------------------------------------------------------- */
/*                              SERVICE DEFINITIONS                           */
/* -------------------------------------------------------------------------- */

// ServiceAccess defines which roles and usernames can trigger a service endpoint.
// FullRoles gain unrestricted access, OwnRoles run under own-only scope, and specific
// usernames can be whitelisted regardless of their role (useful for break-glass users).
type ServiceAccess struct {
	FullRoles []string
	OwnRoles  []string
	Usernames []string
}

// ServiceDefinition describes a custom action endpoint that is not tied to a CRUD
// resource. Handlers are looked up by name inside the controllers package.
type ServiceDefinition struct {
	Name    string
	Method  string
	Path    string
	Handler string
	Access  ServiceAccess
}

// ServiceDefinitions exposes the available service endpoints. These examples illustrate
// how to wire own-scoped actions (current exam), admin overrides (restart exam), and
// lightweight no-op handlers (exampleFunction, exampleFunction2).
var ServiceDefinitions = []ServiceDefinition{
	{
		Name:    "currentExam",
		Method:  "GET",
		Path:    "/services/exams/current",
		Handler: "currentExam",
		Access: ServiceAccess{
			FullRoles: []string{"admin"},
			OwnRoles:  []string{"user"},
		},
	},
	{
		Name:    "restartExam",
		Method:  "POST",
		Path:    "/services/exams/restart",
		Handler: "restartExam",
		Access: ServiceAccess{
			FullRoles: []string{"admin"},
			Usernames: []string{"exam-supervisor"},
		},
	},
	{
		Name:    "exampleFunction",
		Method:  "POST",
		Path:    "/services/example-function",
		Handler: "exampleFunction",
		Access: ServiceAccess{
			FullRoles: []string{"user", "reviewer", "admin"},
		},
	},
	{
		Name:    "exampleFunction2",
		Method:  "GET",
		Path:    "/services/example-function-2",
		Handler: "exampleFunction2",
		Access: ServiceAccess{
			FullRoles: []string{"admin"},
			Usernames: []string{"integration-bot"},
		},
	},
}
