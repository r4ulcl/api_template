package routes

import (
	"log"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/controllers"
	"github.com/r4ulcl/api_template/api/middlewares"
	_ "github.com/r4ulcl/api_template/docs"
	"github.com/r4ulcl/api_template/utils/models"
	httpSwagger "github.com/swaggo/http-swagger"
)

// SetupRouter sets up Gorilla Mux with our handlers and Swagger UI.
// (No Swagger annotations here—each endpoint is documented in its own setup function.)
func SetupRouter(
	baseController *controllers.Controller,
	authController *controllers.AuthController,
	jwtSecret string,
	userGUI string,
) *mux.Router {
	r := mux.NewRouter()

	// 1) CORS preflight middleware makes OPTIONS responses automatic for all registered routes.
	r.Use(mux.CORSMethodMiddleware(r))

	// 2) Swagger UI (no authentication required)
	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// 3) Unprotected auth endpoints (handled by AuthController.Login for POST)
	r.HandleFunc("/login", authController.Login).Methods("POST")

	// 4) “all” subrouter requires a valid JWT
	all := r.NewRoute().Subrouter()
	all.Use(middlewares.AuthMiddleware(jwtSecret))

	// 4) Protected auth endpoints (handled by AuthController.Login for GET/PUT)
	all.HandleFunc("/login", authController.Login).Methods("GET", "PUT")

	// 5) Non-admin resources (GET list with pagination/filters, GET by ID)
	root := "/"
	resources := []string{"example1", "example2", "exampleRelational"}

	// Map resource names → model pointers
	modelMap := map[string]interface{}{
		"user":              &models.User{},
		"example1":          &models.Example1{},
		"example2":          &models.Example2{},
		"exampleRelational": &models.ExampleRelational{},
	}

	setupURLResourceRoutes(all, baseController, authController, root, resources, modelMap, userGUI)

	// 6) Admin-only endpoints (wrap another subrouter with AdminOnly)
	adminOnly := all.NewRoute().Subrouter()
	adminOnly.Use(middlewares.AdminOnly)

	// Admin GET/DELETE
	rootAdmin := "/"
	resourcesAdmin := []string{"user", "example1", "example2", "exampleRelational"}
	setupURLAdminResourceRoutes(adminOnly, baseController, rootAdmin, resourcesAdmin, modelMap, userGUI)

	// Admin POST/PUT/PATCH
	setupBodyAdminResourceRoutes(adminOnly, baseController, rootAdmin, resourcesAdmin, modelMap)

	return r
}

// setupURLResourceRoutes sets up the common GET routes (list + by-ID) for resources.
// @Summary    Retrieve resources (paginated & filterable) or a single resource by ID
// @Tags       resources
// @Description Returns a paginated list of items for the given resource, applying any query-string filters (e.g., ?status=active&category=books). To fetch a single item, include “/{id}”.
// @Param      resource   path     string  true   "Resource type"                                   Enums(example1, example2, exampleRelational)
// @Param      id         path     string  false  "Resource ID (when fetching a single item)"
// @Param      page       query    int     false  "Page number (default: 1)"                         default(1)
// @Param      page_size   query    int     false  "Number of items per page (default: 10)"           default(10)
// @Param      *          query    string  false  "Any other key=value acts as a filter (e.g., ?status=active). Multiple filters allowed."
// @Produce    json
// @Success    200  {object}  map[string]interface{}  "Returns { data: [...], meta: {...}, links: {...} }"
// @Failure    400  {object}  models.ErrorResponse   "Invalid request"
// @Failure    500  {object}  models.ErrorResponse   "Internal server error"
// @Router     /{resource}       [get]
// @Router     /{resource}/{id}  [get]
// @Security   ApiKeyAuth
func setupURLResourceRoutes(
	router *mux.Router,
	controller *controllers.Controller,
	authController *controllers.AuthController,
	root string,
	resources []string,
	modelMap map[string]interface{},
	userGUI string,
) {
	for _, resource := range resources {
		res := resource // capture loop variable
		modelType := modelMap[res]
		resourcePath := root + res

		log.Println("Registering GET routes for resource:", resourcePath)

		// LIST with pagination & filters
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			// Create a slice pointer of the correct type (e.g., *[]Example1)
			slicePtr := reflect.New(reflect.SliceOf(reflect.TypeOf(modelType).Elem())).Interface()
			controller.GetAll(w, r, slicePtr)
		}).Methods("GET")

		// GET BY ID
		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			instancePtr := reflect.New(reflect.TypeOf(modelType)).Interface()
			controller.GetByID(w, r, instancePtr)
		}).Methods("GET")
	}

	if userGUI == "true" {
		// GET /stats (only when GUI is enabled)
		statsPath := root + "stats"
		log.Println("Registering USER GET /stats at:", statsPath)
		router.HandleFunc(statsPath, controller.GetDBStats).Methods("GET")
	}

	// Allow login via PUT/GET (duplicated for GUI)
	router.HandleFunc("/login", authController.Login).Methods("PUT", "GET")
}

// setupURLAdminResourceRoutes sets up the admin GET (list for users) and DELETE routes.
// @Summary    Admin: list users or delete a resource by ID
// @Tags       admin
// @Description If “resource=user”, GET /user returns all users (paginated & filterable). DELETE /{resource}/{id} deletes the specified item.
// @Param      resource   path     string  true   "Resource type"                                   Enums(user, example1, example2, exampleRelational)
// @Param      id         path     string  false  "Resource ID (for delete operations)"
// @Produce    json
// @Success    200  {object}  interface{}       "For GET /user: array of users; for DELETE: { message: \"Deleted successfully\" }"
// @Failure    400  {object}  models.ErrorResponse  "Invalid resource"
// @Failure    403  {object}  models.ErrorResponse  "Forbidden: Admins only"
// @Failure    500  {object}  models.ErrorResponse  "Internal server error"
// @Router     /user       [get]     // only applies if resource=user
// @Router     /{resource}/{id}  [delete]
// @Security   ApiKeyAuth
func setupURLAdminResourceRoutes(
	router *mux.Router,
	controller *controllers.Controller,
	root string,
	resources []string,
	modelMap map[string]interface{},
	userGUI string,
) {
	for _, resource := range resources {
		res := resource
		modelType := modelMap[res]
		resourcePath := root + res

		// If this is “user”, also allow GET /user (list all users)
		if res == "user" {
			log.Println("Registering ADMIN GET /user")
			router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
				if modelType == nil {
					http.Error(w, "Invalid resource", http.StatusBadRequest)
					return
				}
				slicePtr := reflect.New(reflect.SliceOf(reflect.TypeOf(modelType).Elem())).Interface()
				controller.GetAll(w, r, slicePtr)
			}).Methods("GET")
		}

		// DELETE /{resource}/{id}
		log.Println("Registering ADMIN DELETE for:", resourcePath+"/{id}")
		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			controller.Delete(w, r, modelType)
		}).Methods("DELETE")
	}

	log.Println("userGUI", userGUI)

	if userGUI != "true" {
		// GET /stats (only if GUI is disabled)
		statsPath := root + "stats"
		log.Println("Registering ADMIN GET /stats at:", statsPath)
		router.HandleFunc(statsPath, controller.GetDBStats).Methods("GET")
	}
}

// setupBodyAdminResourceRoutes sets up the admin POST, PUT, and PATCH routes.
// @Summary    Admin: create/overwrite or update a resource
// @Tags       admin
// @Description Allows admin to create (POST), overwrite (PUT), or update (PATCH) any resource.
// @Param      resource          path     string                  true   "Resource type"                           Enums(user, example1, example2, exampleRelational)
// @Param      id                path     string                  false  "Resource ID (for PATCH)"
// @Param      defaultRequest    body     models.DefaultRequest   true   "Generic request body for POST/PATCH"
// @Param      example1          body     models.Example1         false  "Example1 object to create/overwrite"
// @Param      example2          body     models.Example2         false  "Example2 object to create/overwrite"
// @Param      ExampleRelational body     models.ExampleRelational false "ExampleRelational to create/overwrite"
// @Accept     json
// @Produce    json
// @Success    201  {object}  interface{}             "Returns the created/updated object"
// @Failure    400  {object}  models.ErrorResponse    "Invalid input JSON"
// @Failure    403  {object}  models.ErrorResponse    "Forbidden: Admins only"
// @Failure    500  {object}  models.ErrorResponse    "Internal server error"
// @Router     /{resource}       [post]
// @Router     /{resource}       [put]
// @Router     /{resource}/{id}  [patch]
// @Security   ApiKeyAuth
func setupBodyAdminResourceRoutes(
	router *mux.Router,
	controller *controllers.Controller,
	root string,
	resources []string,
	modelMap map[string]interface{},
) {
	for _, resource := range resources {
		res := resource
		modelType := modelMap[res]
		resourcePath := root + res

		// POST /{resource} (overwrite=false)
		log.Println("Registering ADMIN POST for:", resourcePath)
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			controller.Create(w, r, modelType, false)
		}).Methods("POST")

		// PUT /{resource} (overwrite=true)
		log.Println("Registering ADMIN PUT for:", resourcePath)
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			controller.Create(w, r, modelType, true)
		}).Methods("PUT")

		// PATCH /{resource}/{id}
		log.Println("Registering ADMIN PATCH for:", resourcePath+"/{id}")
		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			controller.Update(w, r, modelType)
		}).Methods("PATCH")
	}
}
