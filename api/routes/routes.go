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

	if swagger {
		r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
	}

	// Public auth endpoints
	r.HandleFunc("/login", authController.Login).Methods("POST")

	if publicRegister {
		r.HandleFunc("------------------------ /register", authController.Register).Methods("POST")
	}

	// ---------- 1. Anonymous resources ----------
	anon := models.RolePermissions["anonymous"]

	for _, res := range anon.Get {
		registerCRUD(r, baseController, authController, "GET", res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Post {
		registerCRUD(r, baseController, authController, "POST", res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Put {
		registerCRUD(r, baseController, authController, "PUT", res, models.ModelMap[res], nil, nil, true, readLog)
	}
	for _, res := range anon.Patch {
		registerCRUD(r, baseController, authController, "PATCH", res, models.ModelMap[res], nil, nil, false, readLog)
	}
	for _, res := range anon.Delete {
		registerCRUD(r, baseController, authController, "DELETE", res, models.ModelMap[res], nil, nil, false, readLog)
	}

	// ---------- 2. Authenticated resources ----------
	authSub := r.NewRoute().Subrouter()
	authSub.Use(middlewares.AuthMiddleware(jwtSecret, baseController.BC.DB))

	// me endpoints
	// Me endpoints available to any logged in user
	authSub.HandleFunc("/me", authController.Me).Methods("GET", "PATCH")

	// Collect all non-anonymous roles for own access on /me routes
	allAuthRoles := make([]string, 0, len(models.RolePermissions))
	for role := range models.RolePermissions {
		if role != "anonymous" {
			allAuthRoles = append(allAuthRoles, role)
		}
	}
	// Wrap DeleteAPIKey with OwnScopeMiddleware so only owners can delete their key
	authSub.Handle(
		"/me/api-key/{apiKey}",
		middlewares.OwnScopeMiddleware(nil, allAuthRoles)(http.HandlerFunc(authController.DeleteAPIKey)),
	).Methods(http.MethodDelete)

	authSub.Handle(
		"/me/api-key/{apiKey}",
		middlewares.OwnScopeMiddleware(nil, allAuthRoles)(http.HandlerFunc(authController.GenerateAPIKey)),
	).Methods(http.MethodPost)

	// Build method -> resource -> {full, own} role lists
	type both struct {
		full []string
		own  []string
	}
	methodRoles := map[string]map[string]*both{
		"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
	}
	for role, perms := range models.RolePermissions {
		if role == "anonymous" {
			continue
		}

		for _, rsc := range perms.Get {
			entry := methodRoles["GET"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["GET"][rsc] = entry
			}
			entry.full = append(entry.full, role)
		}
		for _, rsc := range perms.Post {
			entry := methodRoles["POST"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["POST"][rsc] = entry
			}
			entry.full = append(entry.full, role)
		}
		for _, rsc := range perms.Put {
			entry := methodRoles["PUT"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["PUT"][rsc] = entry
			}
			entry.full = append(entry.full, role)
		}
		for _, rsc := range perms.Patch {
			entry := methodRoles["PATCH"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["PATCH"][rsc] = entry
			}
			entry.full = append(entry.full, role)
		}
		for _, rsc := range perms.Delete {
			entry := methodRoles["DELETE"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["DELETE"][rsc] = entry
			}
			entry.full = append(entry.full, role)
		}

		// Own permissions
		for _, rsc := range perms.GetOwn {
			entry := methodRoles["GET"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["GET"][rsc] = entry
			}
			entry.own = append(entry.own, role)
		}
		for _, rsc := range perms.PostOwn {
			entry := methodRoles["POST"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["POST"][rsc] = entry
			}
			entry.own = append(entry.own, role)
		}
		for _, rsc := range perms.PutOwn {
			entry := methodRoles["PUT"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["PUT"][rsc] = entry
			}
			entry.own = append(entry.own, role)
		}
		for _, rsc := range perms.PatchOwn {
			entry := methodRoles["PATCH"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["PATCH"][rsc] = entry
			}
			entry.own = append(entry.own, role)
		}
		for _, rsc := range perms.DeleteOwn {
			entry := methodRoles["DELETE"][rsc]
			if entry == nil {
				entry = &both{}
				methodRoles["DELETE"][rsc] = entry
			}
			entry.own = append(entry.own, role)
		}
	}

	// Register every method/resource on authSub with both role sets
	for res, rs := range methodRoles["GET"] {
		registerCRUD(authSub, baseController, authController, "GET", res, models.ModelMap[res], rs.full, rs.own, false, readLog)
	}
	for res, rs := range methodRoles["POST"] {
		registerCRUD(authSub, baseController, authController, "POST", res, models.ModelMap[res], rs.full, rs.own, false, readLog)
	}
	for res, rs := range methodRoles["PUT"] {
		registerCRUD(authSub, baseController, authController, "PUT", res, models.ModelMap[res], rs.full, rs.own, true, readLog)
	}
	for res, rs := range methodRoles["PATCH"] {
		registerCRUD(authSub, baseController, authController, "PATCH", res, models.ModelMap[res], rs.full, rs.own, false, readLog)
	}
	for res, rs := range methodRoles["DELETE"] {
		registerCRUD(authSub, baseController, authController, "DELETE", res, models.ModelMap[res], rs.full, rs.own, false, readLog)
	}

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

	// ---------- 3. /stats endpoint ----------
	if userGUI {
		sub := r.NewRoute().Subrouter()
		sub.Use(middlewares.AuthMiddleware(jwtSecret, baseController.BC.DB))
		sub.HandleFunc("/stats", baseController.GetDBStats).Methods("GET")
	} else {
		sub := r.NewRoute().Subrouter()
		sub.Use(middlewares.AuthMiddleware(jwtSecret, baseController.BC.DB))
		sub.Use(middlewares.RoleMiddleware("admin"))
		sub.HandleFunc("/stats", baseController.GetDBStats).Methods("GET")
	}

	return r
}
