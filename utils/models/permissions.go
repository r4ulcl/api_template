package models

// Permissions holds the slice of resource‐names for each HTTP method.
type Permissions struct {
	Get    []string
	Post   []string
	Put    []string
	Patch  []string
	Delete []string
}

// Existing “anonymous”, “User” (user), and “admin” slices remain, for backward compatibility.
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
// Feel free to add/remove keys here, or change the slices at runtime.
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

// ModelMap (unchanged) still maps resource‐names to model pointers.
var ModelMap = map[string]interface{}{
	"user":              &User{},
	"example1":          &Example1{},
	"example2":          &Example2{},
	"exampleRelational": &ExampleRelational{},
}
