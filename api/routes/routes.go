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

// SetupRouter sets up Gorilla Mux with our handlers and Swagger
// @Summary Login and generate JWT token
// @Description Login using username and password, and return a JWT token for authorized access
// @Tags authentication
// @Accept json
// @Produce json
// @Param body body models.LoginRequest true "Login request with username and password"
// @Success 200 {object} models.JWTResponse
// @Failure 400 {string} string "Invalid input"
// @Failure 401 {Object} models.ErrorResponse "Unauthorized"
// @Router /login [post]
// @Router /login [get]
// @Router /login [put]
// @security ApiKeyAuth
func SetupRouter(baseController *controllers.Controller, authController *controllers.AuthController,
	jwtSecret string, userGUI string,
) *mux.Router {
	r := mux.NewRouter()

	// 2) Before registering any routes, tell the router to handle OPTIONS for us:
	//    whenever you register GET/POST/PUT/DELETE/etc. on a path, CORSMethodMiddleware
	//    will generate a matching OPTIONS + “Allow:” header automatically.
	r.Use(mux.CORSMethodMiddleware(r))

	// 3) (Optional) If you want a single catch-all OPTIONS for anything not covered:
	//    The middleware above is typically enough. But if you still see a 405 on
	//    some nested subrouter, uncomment the following two lines and place them *after*
	//    you’ve registered Swagger and /login but *before* you create subrouters.
	//
	// r.PathPrefix("/").Methods("OPTIONS").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	//     w.WriteHeader(http.StatusOK)
	// })

	// 4) Swagger UI (no JWT required)
	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// 5) Unprotected auth endpoints
	r.HandleFunc("/login", authController.Login).Methods("POST")

	// 6) “all” is the subrouter for any endpoint that *requires* a valid JWT
	all := r.NewRoute().Subrouter()
	all.Use(middlewares.AuthMiddleware(jwtSecret))

	// 7) Non-admin resources (GET list, GET by id)
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

	// 8) Admin-only endpoints (wrap another subrouter with AdminOnly)
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

// setupURLResourceRoutes sets up the common routes for CRUD operations for resources
// @Summary Setup GET resource routes
// @Tags user
// @Description Setup routes for CRUD operations on resources like users, servers, employees, etc.
// @Param resource path string true "Resource type" Enums(example1, example2, exampleRelational)
// @Param id path string false "Resource ID (for operations on specific resources)"
// @Router /{resource} [get]
// @Router /{resource}/{id} [get]
// @security ApiKeyAuth
func setupURLResourceRoutes(router *mux.Router, controller *controllers.Controller, authController *controllers.AuthController,
	root string, resources []string, modelMap map[string]interface{}, userGUI string,
) {
	for _, resource := range resources {
		res := resource // capture loop variable
		modelType := modelMap[res]
		resourcePath := root + res

		log.Println("Registering GET routes for resource:", resourcePath)

		// LIST
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)
				return
			}
			// create a slice pointer of the correct type (e.g. *[]Example1)
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
		// GET /stats
		statsPath := root + "stats"
		log.Println("Registering USER GET /stats at:", statsPath)
		router.HandleFunc(statsPath, controller.GetDBStats).Methods("GET")
	}

	//Get user info
	router.HandleFunc("/login", authController.Login).Methods("PUT", "GET")
}

// setupURLAdminResourceRoutes sets up the admin routes for resources like /user, /server, /employee, /group, etc.
// @Summary Setup admin routes
// @Tags admin
// @Description Setup routes for administrative resources like users, servers, employees, etc.
// @Param resource path string true "Resource type" Enums(user, example1, example2, exampleRelational)
// @Param id path string false "Resource ID (for operations on specific resources)"
// @Router /user [get]                     // GET route: No body parameter
// @Router /{resource}/{id} [delete]       // DELETE route: No body parameter
// @security ApiKeyAuth
// @security ApiKeyAuth.
func setupURLAdminResourceRoutes(router *mux.Router, controller *controllers.Controller,
	root string, resources []string, modelMap map[string]interface{}, userGUI string,
) {
	for _, resource := range resources {
		res := resource
		modelType := modelMap[res]
		resourcePath := root + res

		// If this is “user”, also allow GET /user to list all users
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
		// GET /stats
		statsPath := root + "stats"
		log.Println("Registering ADMIN GET /stats at:", statsPath)
		router.HandleFunc(statsPath, controller.GetDBStats).Methods("GET")
	}
}

// setupBodyAdminResourceRoutes sets up the admin routes for resources like /users, /servers, /employee, /groups, etc.
// @Summary Setup admin routes
// @Tags admin
// @Description Setup routes for administrative resources like users, servers, employees, etc.
// @Param resource path string true "Resource type" Enums(user, example1, example2, exampleRelational)
// @Param id path string false "Resource ID (for operations on specific resources)"
// @security ApiKeyAuth
// @Router /{resource} [post]
// @Router /{resource} [put]
// @Router /{resource}/{id} [patch]
// @Param defaultRequest body models.DefaultRequest true "JSON request body for POST and PATCH operations"
// @param example1 body models.Example1 false "Example1 object to create"
// @param example2 body models.Example2 false "Example2 object to create"
// @param ExampleRelational body models.ExampleRelational false "ExampleRelational object to create".
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
