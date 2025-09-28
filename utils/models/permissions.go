package models

// Permissions holds the slice of resource‐names for each HTTP method.
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

// Existing 3 roles anonymous, User, and admin
// Edit this arrays to allow access to roles to endpoints
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

var UserGetResources = []string{"example2"}
var UserGetResourcesOwn = []string{"example1", "exampleRelational"}
var UserPostResources = []string{"example2"}
var UserPostResourcesOwn = []string{"example1", "exampleRelational"}
var UserPutResources = []string{}
var UserPutResourcesOwn = []string{}
var UserPatchResources = []string{}
var UserPatchResourcesOwn = []string{}
var UserDeleteResources = []string{}
var UserDeleteResourcesOwn = []string{}

var AdminGetResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminGetResourcesOwn = []string{}
var AdminPostResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPostResourcesOwn = []string{}
var AdminPutResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPutResourcesOwn = []string{}
var AdminPatchResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPatchResourcesOwn = []string{}
var AdminDeleteResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminDeleteResourcesOwn = []string{}

// RolePermissions ties a role‐name (string) to its Permissions.
// Feel free to add/remove keys here.
// The router will pick up any change automatically.
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
