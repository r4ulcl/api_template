// routes/router.go
package routes

import (
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/controllers"
	"github.com/r4ulcl/api_template/api/middlewares"
	_ "github.com/r4ulcl/api_template/docs"
	"github.com/r4ulcl/api_template/utils/models"
	httpSwagger "github.com/swaggo/http-swagger"
)

// -----------------------------------------------------------------------------
// Helper: register one CRUD route (list, by-ID, create, update, …)
// -----------------------------------------------------------------------------
func registerCRUD(
	router *mux.Router,
	controller *controllers.Controller,
	verb string, // "GET", "POST", "PUT", "PATCH", "DELETE"
	resource string, // e.g. "example1"
	modelType interface{},
	roles []string, // nil|empty ⇒ no RoleMiddleware
	overwrite bool, // only used for PUT
) {
	base := "/" + resource // collection endpoint
	item := base + "/{id}" // single-item endpoint
	wrap := func(h http.Handler) http.Handler {
		if len(roles) == 0 {
			return h
		}
		return middlewares.RoleMiddleware(roles...)(h)
	}

	switch verb {
	// ---------- GET (list + by-ID) ----------
	case "GET":
		list := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slicePtr := reflect.New(reflect.SliceOf(reflect.TypeOf(modelType).Elem())).Interface()
			controller.GetAll(w, r, slicePtr)
		})
		byID := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			instancePtr := reflect.New(reflect.TypeOf(modelType)).Interface()
			controller.GetByID(w, r, instancePtr)
		})

		router.Handle(base, wrap(list)).Methods("GET")
		router.Handle(item, wrap(byID)).Methods("GET")

	// ---------- POST (collection) ----------
	case "POST":
		create := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			controller.Create(w, r, modelType, false)
		})
		router.Handle(base, wrap(create)).Methods("POST")

	// ---------- PUT (collection) ----------
	case "PUT":
		put := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			controller.Create(w, r, modelType, overwrite)
		})
		router.Handle(base, wrap(put)).Methods("PUT")

	// ---------- PATCH (single item) ----------
	case "PATCH":
		patch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			controller.Update(w, r, modelType)
		})
		router.Handle(item, wrap(patch)).Methods("PATCH")

	// ---------- DELETE (single item) ----------
	case "DELETE":
		del := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			controller.Delete(w, r, modelType)
		})
		router.Handle(item, wrap(del)).Methods("DELETE")
	}
}

// -----------------------------------------------------------------------------
// Public entry: SetupRouter
// -----------------------------------------------------------------------------
func SetupRouter(
	baseController *controllers.Controller,
	authController *controllers.AuthController,
	jwtSecret string,
	userGUI, swagger bool,
) *mux.Router {
	r := mux.NewRouter()
	r.Use(mux.CORSMethodMiddleware(r))

	if swagger {
		r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
	}

	// Unprotected login
	r.HandleFunc("/login", authController.Login).Methods("POST")

	// ---------- 1. Anonymous resources --------------------------------------
	anon := models.RolePermissions["anonymous"]

	for _, res := range anon.Get {
		registerCRUD(r, baseController, "GET", res, models.ModelMap[res], nil, false)
	}
	for _, res := range anon.Post {
		registerCRUD(r, baseController, "POST", res, models.ModelMap[res], nil, false)
	}
	for _, res := range anon.Put {
		registerCRUD(r, baseController, "PUT", res, models.ModelMap[res], nil, true)
	}
	for _, res := range anon.Patch {
		registerCRUD(r, baseController, "PATCH", res, models.ModelMap[res], nil, false)
	}
	for _, res := range anon.Delete {
		registerCRUD(r, baseController, "DELETE", res, models.ModelMap[res], nil, false)
	}

	// ---------- 2. Authenticated resources ----------------------------------
	// Single subrouter with AuthMiddleware; RoleMiddleware is per-route.
	authSub := r.NewRoute().Subrouter()
	authSub.Use(middlewares.AuthMiddleware(jwtSecret))

	// Build: method → resource → []roles  (collecting all non-anonymous roles)
	methodRoles := map[string]map[string][]string{
		"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
	}
	for role, perms := range models.RolePermissions {
		if role == "anonymous" {
			continue
		}
		for _, r := range perms.Get {
			methodRoles["GET"][r] = append(methodRoles["GET"][r], role)
		}
		for _, r := range perms.Post {
			methodRoles["POST"][r] = append(methodRoles["POST"][r], role)
		}
		for _, r := range perms.Put {
			methodRoles["PUT"][r] = append(methodRoles["PUT"][r], role)
		}
		for _, r := range perms.Patch {
			methodRoles["PATCH"][r] = append(methodRoles["PATCH"][r], role)
		}
		for _, r := range perms.Delete {
			methodRoles["DELETE"][r] = append(methodRoles["DELETE"][r], role)
		}
	}

	// Register every method/resource exactly once on authSub
	for res, roles := range methodRoles["GET"] {
		registerCRUD(authSub, baseController, "GET", res, models.ModelMap[res], roles, false)
	}
	for res, roles := range methodRoles["POST"] {
		registerCRUD(authSub, baseController, "POST", res, models.ModelMap[res], roles, false)
	}
	for res, roles := range methodRoles["PUT"] {
		registerCRUD(authSub, baseController, "PUT", res, models.ModelMap[res], roles, true)
	}
	for res, roles := range methodRoles["PATCH"] {
		registerCRUD(authSub, baseController, "PATCH", res, models.ModelMap[res], roles, false)
	}
	for res, roles := range methodRoles["DELETE"] {
		registerCRUD(authSub, baseController, "DELETE", res, models.ModelMap[res], roles, false)
	}

	// ---------- 3. /stats endpoint ------------------------------------------
	if userGUI {
		sub := r.NewRoute().Subrouter()
		sub.Use(middlewares.AuthMiddleware(jwtSecret))
		sub.HandleFunc("/stats", baseController.GetDBStats).Methods("GET")
	} else {
		sub := r.NewRoute().Subrouter()
		sub.Use(middlewares.AuthMiddleware(jwtSecret))
		sub.Use(middlewares.RoleMiddleware("admin"))
		sub.HandleFunc("/stats", baseController.GetDBStats).Methods("GET")
	}

	return r
}
