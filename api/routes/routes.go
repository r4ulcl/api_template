package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/controllers"
	"github.com/r4ulcl/api_template/api/middlewares"
	_ "github.com/r4ulcl/api_template/docs"
	"github.com/r4ulcl/api_template/utils/models"
	httpSwagger "github.com/swaggo/http-swagger"
)

// -----------------------------------------------------------------------------
// Helper: register one CRUD route (list, by-ID, create, update, delete) with Own support
// Now also aware of /user so POST and PUT use AuthController.Register
// -----------------------------------------------------------------------------
func registerCRUD(
	router *mux.Router,
	baseController *controllers.Controller,
	authController *controllers.AuthController,
	verb string, // "GET", "POST", "PUT", "PATCH", "DELETE"
	resource string, // e.g. "example1"
	modelType interface{}, // pointer to model
	fullRoles []string, // roles with full access
	ownRoles []string, // roles limited to own data
	overwrite bool, // only used for PUT
	readLog bool, // to log READ requests
) {
	base := "/" + resource
	item := base + "/{id}"

	wrap := func(h http.Handler) http.Handler {
		if len(fullRoles) == 0 && len(ownRoles) == 0 {
			return h
		}
		return middlewares.OwnScopeMiddleware(fullRoles, ownRoles)(h)
	}

	switch verb {
	case "GET":
		// Normalize types once
		mt := reflect.TypeOf(modelType) // maybe *T or T
		elem := mt
		if mt.Kind() == reflect.Ptr {
			elem = mt.Elem() // T
		}

		list := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// *([]T)
			slicePtr := reflect.New(reflect.SliceOf(elem)).Interface()
			baseController.GetAll(w, r, slicePtr, readLog)
		})

		byID := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// *T (never **T)
			instancePtr := reflect.New(elem).Interface()
			baseController.GetByID(w, r, instancePtr, readLog)
		})

		router.Handle(base, wrap(list)).Methods("GET")
		router.Handle(item, wrap(byID)).Methods("GET")

	case "POST":
		var h http.HandlerFunc
		if resource == "user" && authController != nil {
			// Creation of users goes through Register
			h = authController.Register
		} else {
			h = func(w http.ResponseWriter, r *http.Request) {
				baseController.Create(w, r, modelType, false)
			}
		}
		router.Handle(base, wrap(h)).Methods("POST")

	case "PUT":
		var h http.HandlerFunc
		if resource == "user" && authController != nil {
			// Admin managed creation or idempotent upsert of users goes through Register
			h = authController.Register
		} else {
			h = func(w http.ResponseWriter, r *http.Request) {
				baseController.Create(w, r, modelType, overwrite)
			}
		}
		router.Handle(base, wrap(h)).Methods("PUT")

	case "PATCH":
		patch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			baseController.Update(w, r, modelType)
		})
		router.Handle(item, wrap(patch)).Methods("PATCH")

	case "DELETE":
		del := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			baseController.Delete(w, r, modelType)
		})
		router.Handle(item, wrap(del)).Methods("DELETE")
	}
}

func serviceAccessMiddleware(access models.ServiceAccess) func(http.Handler) http.Handler {
	hasRoleRestrictions := len(access.FullRoles) > 0 || len(access.OwnRoles) > 0
	hasUserRestrictions := len(access.Usernames) > 0

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			usernameVal := ctx.Value(middlewares.ContextUserID)
			username, _ := usernameVal.(string)
			roleVal := ctx.Value(middlewares.ContextRole)
			role, _ := roleVal.(string)

			if strings.TrimSpace(username) == "" || strings.TrimSpace(role) == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing authentication context"})
				return
			}

			if !hasRoleRestrictions && !hasUserRestrictions {
				ctx = context.WithValue(ctx, middlewares.ContextOwnOnly, false)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if contains(access.Usernames, username) {
				ctx = context.WithValue(ctx, middlewares.ContextOwnOnly, false)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			isFull := contains(access.FullRoles, role)
			isOwn := contains(access.OwnRoles, role)

			if !isFull && !isOwn {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Forbidden: insufficient permissions"})
				return
			}

			ctx = context.WithValue(ctx, middlewares.ContextOwnOnly, isOwn && !isFull)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

type roleBuckets struct {
	full []string
	own  []string
}

// -----------------------------------------------------------------------------
// Public entry: SetupRouter
// -----------------------------------------------------------------------------
func SetupRouter(
	baseController *controllers.Controller,
	authController *controllers.AuthController,
	jwtSecret string,
	publicRegister, userGUI, swagger, readLog bool,
) *mux.Router {
	r := mux.NewRouter()
	r.Use(mux.CORSMethodMiddleware(r))

	configureSwagger(r, swagger)
	registerPublicAuthRoutes(r, authController, publicRegister)
	registerAnonymousResources(r, baseController, authController, readLog)

	authSub := newAuthSubrouter(r, jwtSecret, baseController)
	registerMeRoutes(authSub, authController)
	registerRoleProtectedResources(authSub, baseController, authController, readLog)
	registerServiceDefinitions(authSub, baseController)
	registerStatsRoute(r, baseController, jwtSecret, userGUI)

	return r
}

func configureSwagger(r *mux.Router, enabled bool) {
	if !enabled {
		return
	}
	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
}

func registerPublicAuthRoutes(r *mux.Router, authController *controllers.AuthController, publicRegister bool) {
	r.HandleFunc("/login", authController.Login).Methods("POST")
	if publicRegister {
		r.HandleFunc("------------------------ /register", authController.Register).Methods("POST")
	}
}

func registerAnonymousResources(r *mux.Router, baseController *controllers.Controller, authController *controllers.AuthController, readLog bool) {
	anon := models.RolePermissions["anonymous"]

	for _, res := range anon.Get {
		registerCRUD(r, baseController, authController, http.MethodGet, res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Post {
		registerCRUD(r, baseController, authController, http.MethodPost, res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Put {
		registerCRUD(r, baseController, authController, http.MethodPut, res, models.ModelMap[res], nil, nil, true, readLog)
	}
	for _, res := range anon.Patch {
		registerCRUD(r, baseController, authController, http.MethodPatch, res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Delete {
		registerCRUD(r, baseController, authController, http.MethodDelete, res, models.ModelMap[res], nil, nil, false, readLog)
	}
}

func newAuthSubrouter(r *mux.Router, jwtSecret string, baseController *controllers.Controller) *mux.Router {
	authSub := r.NewRoute().Subrouter()
	authSub.Use(middlewares.AuthMiddleware(jwtSecret, baseController.BC.DB))
	return authSub
}

func registerMeRoutes(authSub *mux.Router, authController *controllers.AuthController) {
	authSub.HandleFunc("/me", authController.Me).Methods(http.MethodGet, http.MethodPatch)
	roles := collectNonAnonymousRoles()
	authSub.Handle(
		"/me/api-key/{apiKey}",
		middlewares.OwnScopeMiddleware(nil, roles)(http.HandlerFunc(authController.DeleteAPIKey)),
	).Methods(http.MethodDelete)
	authSub.Handle(
		"/me/api-key",
		middlewares.OwnScopeMiddleware(nil, roles)(http.HandlerFunc(authController.GenerateAPIKey)),
	).Methods(http.MethodPost)
}

func collectNonAnonymousRoles() []string {
	roles := make([]string, 0, len(models.RolePermissions))
	for role := range models.RolePermissions {
		if role == "anonymous" {
			continue
		}
		roles = append(roles, role)
	}
	return roles
}

func registerRoleProtectedResources(authSub *mux.Router, baseController *controllers.Controller, authController *controllers.AuthController, readLog bool) {
	matrix := buildRoleMatrix()
	for method, resources := range matrix {
		registerMethodResources(method, resources, authSub, baseController, authController, readLog)
	}
}

func buildRoleMatrix() map[string]map[string]*roleBuckets {
	matrix := map[string]map[string]*roleBuckets{
		http.MethodGet:    {},
		http.MethodPost:   {},
		http.MethodPut:    {},
		http.MethodPatch:  {},
		http.MethodDelete: {},
	}

	for role, perms := range models.RolePermissions {
		if role == "anonymous" {
			continue
		}

		appendResource := func(method string, resources []string, target func(*roleBuckets) *[]string) {
			for _, res := range resources {
				entry := matrix[method][res]
				if entry == nil {
					entry = &roleBuckets{}
					matrix[method][res] = entry
				}
				bucket := target(entry)
				*bucket = append(*bucket, role)
			}
		}

		appendResource(http.MethodGet, perms.Get, func(rb *roleBuckets) *[]string { return &rb.full })
		appendResource(http.MethodPost, perms.Post, func(rb *roleBuckets) *[]string { return &rb.full })
		appendResource(http.MethodPut, perms.Put, func(rb *roleBuckets) *[]string { return &rb.full })
		appendResource(http.MethodPatch, perms.Patch, func(rb *roleBuckets) *[]string { return &rb.full })
		appendResource(http.MethodDelete, perms.Delete, func(rb *roleBuckets) *[]string { return &rb.full })

		appendResource(http.MethodGet, perms.GetOwn, func(rb *roleBuckets) *[]string { return &rb.own })
		appendResource(http.MethodPost, perms.PostOwn, func(rb *roleBuckets) *[]string { return &rb.own })
		appendResource(http.MethodPut, perms.PutOwn, func(rb *roleBuckets) *[]string { return &rb.own })
		appendResource(http.MethodPatch, perms.PatchOwn, func(rb *roleBuckets) *[]string { return &rb.own })
		appendResource(http.MethodDelete, perms.DeleteOwn, func(rb *roleBuckets) *[]string { return &rb.own })
	}

	return matrix
}

func registerMethodResources(
	method string,
	resources map[string]*roleBuckets,
	router *mux.Router,
	baseController *controllers.Controller,
	authController *controllers.AuthController,
	readLog bool,
) {
	for resource, roles := range resources {
		overwrite := method == http.MethodPut
		registerCRUD(router, baseController, authController, method, resource, models.ModelMap[resource], roles.full, roles.own, overwrite, readLog)
	}
}

func registerServiceDefinitions(authSub *mux.Router, baseController *controllers.Controller) {
	for _, svc := range models.ServiceDefinitions {
		handler, ok := controllers.GetServiceHandler(svc.Handler, baseController)
		if !ok {
			continue
		}

		method := strings.ToUpper(strings.TrimSpace(svc.Method))
		if method == "" {
			method = http.MethodPost
		}

		h := serviceAccessMiddleware(svc.Access)(handler)
		authSub.Handle(svc.Path, h).Methods(method)
	}
}

func registerStatsRoute(r *mux.Router, baseController *controllers.Controller, jwtSecret string, userGUI bool) {
	sub := r.NewRoute().Subrouter()
	sub.Use(middlewares.AuthMiddleware(jwtSecret, baseController.BC.DB))
	if !userGUI {
		sub.Use(middlewares.RoleMiddleware("admin"))
	}
	sub.HandleFunc("/stats", baseController.GetDBStats).Methods(http.MethodGet)
}
