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
// @security ApiKeyAuth
func SetupRouter(baseController *controllers.Controller, authController *controllers.AuthController,
	jwtSecret string,
) *mux.Router {
	r := mux.NewRouter()

	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	r.HandleFunc("/login", authController.Login).Methods("POST")

	// General API subrouter with authentication middleware
	all := r.NewRoute().Subrouter()
	all.Use(middlewares.AuthMiddleware(jwtSecret)) // Protect API routes

	// Generic route setup for resources like /users, /servers, /employee, /groups, etc.
	root := "/"
	resources := []string{"example1", "example2", "exampleRelational"}
	// Define a map to associate resource names with the correct model type
	modelMap := map[string]interface{}{
		"user":              &models.User{},
		"example1":          &models.Example1{},
		"example2":          &models.Example2{},
		"exampleRelational": &models.ExampleRelational{},
	}
	setupURLResourceRoutes(all, baseController, root, resources, modelMap)

	// Admin-only subrouter
	adminOnly := all.NewRoute().Subrouter()
	adminOnly.Use(middlewares.AdminOnly)

	// Generic admin route setup for resources
	rootAdmin := "/"
	resourcesAdmin := []string{"user", "example1", "example2", "exampleRelational"}
	// Separated to have different Swagger comments
	setupURLAdminResourceRoutes(adminOnly, baseController, rootAdmin, resourcesAdmin, modelMap)
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
func setupURLResourceRoutes(router *mux.Router, controller *controllers.Controller,
	root string, resources []string, modelMap map[string]interface{},
) {
	for _, resource := range resources {
		resourcePath := root + resource
		log.Println("resourcePath setupResourceRoutes", resourcePath)

		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}

			// Ensure modelType is a pointer to a slice (e.g., *[]models.User)
			sliceValue := reflect.New(reflect.SliceOf(reflect.TypeOf(modelType).Elem())).Interface()

			// Call GetAll with the correct slice reference
			controller.GetAll(w, r, sliceValue)
		}).Methods("GET")

		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}

			// Call GetByID with the correct model type
			controller.GetByID(w, r, reflect.New(reflect.TypeOf(modelType)).Interface())
		}).Methods("GET")
	}
}

// setupURLAdminResourceRoutes sets up the admin routes for resources like /users, /servers, /employee, /groups, etc.
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
	root string, resources []string, modelMap map[string]interface{},
) {
	for _, resource := range resources {
		resourcePath := root + resource

		// Admin POST route to create a new resource (special handling for "users")
		if resource == "user" {
			router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
				modelType := modelMap[resource]
				if modelType == nil {
					http.Error(w, "Invalid resource", http.StatusBadRequest)

					return
				}
				sliceValue := reflect.New(reflect.SliceOf(reflect.TypeOf(modelType).Elem())).Interface()
				controller.GetAll(w, r, sliceValue)
			}).Methods("GET")
		}

		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}
			controller.Delete(w, r, modelType)
		}).Methods("DELETE")
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
// @param example2 body models.Example2 false "Example2 object to create".
func setupBodyAdminResourceRoutes(router *mux.Router, controller *controllers.Controller,
	root string, resources []string, modelMap map[string]interface{},
) {
	for _, resource := range resources {
		resourcePath := root + resource

		// Admin POST route to create a new resource
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}
			overwrite := false
			controller.Create(w, r, modelType, overwrite)
		}).Methods("POST")

		// Admin POST route to create a new resource
		router.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}
			overwrite := true
			controller.Create(w, r, modelType, overwrite)
		}).Methods("PUT")

		router.HandleFunc(resourcePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			modelType := modelMap[resource]
			if modelType == nil {
				http.Error(w, "Invalid resource", http.StatusBadRequest)

				return
			}
			controller.Update(w, r, modelType)
		}).Methods("PATCH")
	}
}
