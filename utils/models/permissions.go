package models

// Permissions holds the slice of resource‐names for each HTTP method.
type Permissions struct {
	Get    []string
	Post   []string
	Put    []string
	Patch  []string
	Delete []string
}

// Existing 3 roles anonymous, User, and admin
// Edit this arrays to allow access to roles to endpoints
var AnonymousGetResources = []string{}
var AnonymousPostResources = []string{}
var AnonymousPutResources = []string{}
var AnonymousPatchResources = []string{}
var AnonymousDeleteResources = []string{}

var UserGetResources = []string{"example1", "example2", "exampleRelational"}
var UserPostResources = []string{}
var UserPutResources = []string{}
var UserPatchResources = []string{}
var UserDeleteResources = []string{}

var AdminGetResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPostResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPutResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminPatchResources = []string{"user", "example1", "example2", "exampleRelational"}
var AdminDeleteResources = []string{"user", "example1", "example2", "exampleRelational"}

// RolePermissions ties a role‐name (string) to its Permissions.
// Feel free to add/remove keys here.
// The router will pick up any change automatically.
var RolePermissions = map[string]Permissions{
	"anonymous": {
		Get:    AnonymousGetResources,
		Post:   AnonymousPostResources,
		Put:    AnonymousPutResources,
		Patch:  AnonymousPatchResources,
		Delete: AnonymousDeleteResources,
	},
	"user": {
		Get:    UserGetResources,
		Post:   UserPostResources,
		Put:    UserPutResources,
		Patch:  UserPatchResources,
		Delete: UserDeleteResources,
	},
	"admin": {
		Get:    AdminGetResources,
		Post:   AdminPostResources,
		Put:    AdminPutResources,
		Patch:  AdminPatchResources,
		Delete: AdminDeleteResources,
	},
}
