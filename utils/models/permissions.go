package models

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// Permissions defines which resources a role can access for each HTTP verb and scope.
type Permissions struct {
	Get       []string `json:"get"`
	GetOwn    []string `json:"getOwn"`
	Post      []string `json:"post"`
	PostOwn   []string `json:"postOwn"`
	Put       []string `json:"put"`
	PutOwn    []string `json:"putOwn"`
	Patch     []string `json:"patch"`
	PatchOwn  []string `json:"patchOwn"`
	Delete    []string `json:"delete"`
	DeleteOwn []string `json:"deleteOwn"`
}

// ServiceAccess defines which roles and usernames can trigger a service endpoint.
// FullRoles gain unrestricted access, OwnRoles run under own-only scope, and specific
// usernames can be whitelisted regardless of their role (useful for break-glass users).
type ServiceAccess struct {
	FullRoles []string `json:"fullRoles"`
	OwnRoles  []string `json:"ownRoles"`
	Usernames []string `json:"usernames"`
}

// ServiceDefinition describes a custom action endpoint that is not tied to a CRUD
// resource. Handlers are looked up by name inside the controllers package.
type ServiceDefinition struct {
	Name    string        `json:"name"`
	Method  string        `json:"method"`
	Path    string        `json:"path"`
	Handler string        `json:"handler"`
	Access  ServiceAccess `json:"access"`
}

const (
	permissionsEnvKey = "PERMISSIONS_FILE"
	servicesEnvKey    = "SERVICE_DEFINITIONS_FILE"

	defaultPermissionsFile = "example_tables/permissions.json"
	defaultServicesFile    = "example_tables/services.json"
)

var (
	// RolePermissions maps role → permission set.
	RolePermissions = make(map[string]Permissions)
	// ServiceDefinitions exposes the configured service endpoints.
	ServiceDefinitions []ServiceDefinition

	defaultRolePermissionsJSON = []byte(`{
	  "anonymous": {
	    "get": [],
	    "getOwn": [],
	    "post": [],
	    "postOwn": [],
	    "put": [],
	    "putOwn": [],
	    "patch": [],
	    "patchOwn": [],
	    "delete": [],
	    "deleteOwn": []
	  },
	  "user": {
	    "get": ["example2"],
	    "getOwn": ["example1", "exampleRelational", "auditLog", "apiKey"],
	    "post": ["example2"],
	    "postOwn": ["example1", "exampleRelational", "apiKey"],
	    "put": [],
	    "putOwn": ["example1", "example2", "exampleRelational"],
	    "patch": [],
	    "patchOwn": ["example1", "example2", "exampleRelational"],
	    "delete": [],
	    "deleteOwn": ["example1", "example2", "exampleRelational", "apiKey"]
	  },
	  "reviewer": {
	    "get": ["example1", "example2", "exampleRelational"],
	    "getOwn": [],
	    "post": [],
	    "postOwn": [],
	    "put": [],
	    "putOwn": [],
	    "patch": [],
	    "patchOwn": [],
	    "delete": [],
	    "deleteOwn": []
	  },
	  "admin": {
	    "get": ["auditLog", "user", "example1", "example2", "exampleRelational", "apiKey"],
	    "getOwn": [],
	    "post": ["user", "example1", "example2", "exampleRelational", "apiKey"],
	    "postOwn": [],
	    "put": ["user", "example1", "example2", "exampleRelational"],
	    "putOwn": [],
	    "patch": ["user", "example1", "example2", "exampleRelational"],
	    "patchOwn": [],
	    "delete": ["user", "example1", "example2", "exampleRelational", "apiKey"],
	    "deleteOwn": []
	  }
	}`)

	defaultServiceDefinitionsJSON = []byte(`[
	  {
	    "name": "currentExam",
	    "method": "GET",
	    "path": "/services/exams/current",
	    "handler": "currentExam",
	    "access": {
	      "fullRoles": ["admin"],
	      "ownRoles": ["user"],
	      "usernames": []
	    }
	  },
	  {
	    "name": "restartExam",
	    "method": "POST",
	    "path": "/services/exams/restart",
	    "handler": "restartExam",
	    "access": {
	      "fullRoles": ["admin"],
	      "ownRoles": [],
	      "usernames": ["exam-supervisor"]
	    }
	  },
	  {
	    "name": "exampleFunction",
	    "method": "POST",
	    "path": "/services/example-function",
	    "handler": "exampleFunction",
	    "access": {
	      "fullRoles": ["user", "reviewer", "admin"],
	      "ownRoles": [],
	      "usernames": []
	    }
	  },
	  {
	    "name": "exampleFunction2",
	    "method": "GET",
	    "path": "/services/example-function-2",
	    "handler": "exampleFunction2",
	    "access": {
	      "fullRoles": ["admin"],
	      "ownRoles": [],
	      "usernames": ["integration-bot"]
	    }
	  }
	]`)
)

func init() {
	if err := LoadRolePermissions(); err != nil {
		log.Printf("models: %v", err)
	}
	if err := LoadServiceDefinitions(); err != nil {
		log.Printf("models: %v", err)
	}
}

// LoadRolePermissions refreshes RolePermissions from disk or embedded defaults.
func LoadRolePermissions() error {
	data, err := readConfig(permissionsEnvKey, defaultPermissionsFile)
	if err != nil {
		return err
	}

	perms := make(map[string]Permissions)
	if err := json.Unmarshal(data, &perms); err != nil {
		return fmt.Errorf("models: parse role permissions: %w", err)
	}
	RolePermissions = perms
	return nil
}

// LoadServiceDefinitions refreshes ServiceDefinitions from disk or embedded defaults.
func LoadServiceDefinitions() error {
	data, err := readConfig(servicesEnvKey, defaultServicesFile)
	if err != nil {
		return err
	}

	defs := make([]ServiceDefinition, 0)
	if err := json.Unmarshal(data, &defs); err != nil {
		return fmt.Errorf("models: parse service definitions: %w", err)
	}
	ServiceDefinitions = defs
	return nil
}

func readConfig(envKey, defaultPath string) ([]byte, error) {
	if envPath, ok := os.LookupEnv(envKey); ok {
		data, err := os.ReadFile(envPath)
		if err != nil {
			return nil, fmt.Errorf("models: %s=%s could not be read: %w", envKey, envPath, err)
		}
		return data, nil
	}

	if data, err := os.ReadFile(defaultPath); err == nil {
		return data, nil
	}

	return nil, fmt.Errorf("models: %s not found and no embedded fallback; set %s", defaultPath, envKey)
}
