package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/r4ulcl/api_template/api/controllers"
	"github.com/r4ulcl/api_template/api/routes"
	"github.com/r4ulcl/api_template/database"
	_ "github.com/r4ulcl/api_template/docs"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
)

// @title Admin API Documentation
// @version 1.0
// @contact.name r4ulcl
// @description This is a sample API for managing administrative resources like users, servers, employees, groups, etc.
// @termsOfService http://yourdomain.com/terms/

// @contact.name API Support
// @contact.url http://yourdomain.com/support
// @contact.email support@yourdomain.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @BasePath /
// @schemes http https
// @Security ApiKeyAuth
// @securityDefinitions.apikey ApiKeyAuth
// @SecurityScheme ApiKeyAuth
// @in header
// @name Authorization
// @description JWT to login

// main is the entry point of the application.
// It loads the configuration, connects to the database,
// creates a default admin user, initializes controllers,
// sets up the router, and starts the HTTP server.
func main() {
	// Load application configuration
	cfg := utils.LoadConfig()

	// Connect to the database
	database.ConnectDB(cfg)

	// Initialize BaseController (holds the DB instance)
	baseController := &database.BaseController{DB: database.DB}

	// Initialize controllers
	authController := &controllers.AuthController{
		Secret: cfg.JWTSecret,
		BC:     baseController,
	}
	controller := &controllers.Controller{BC: baseController}

	// Create an initial admin user if not already present
	username := "admin"
	adminUser := &models.User{
		Username: username,
		Role:     models.AdminRole,
		Password: cfg.AdminPassword,
	}

	createdUser, err := authController.RegisterUser(adminUser)
	if err != nil {
		log.Printf("Error creating admin user: %v\n", err)
	} else {
		log.Printf("Admin user created: %+v\n", createdUser)
	}

	// Build the router (this already installs CORSMethodMiddleware internally)
	r := routes.SetupRouter(controller, authController, cfg.JWTSecret)

	// Wrap the router in gorilla/handlers.CORS so that:
	// 1) every response (including auto‐OPTIONS) carries the CORS headers, and
	// 2) the preflight (OPTIONS) will be allowed through.
	corsHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}), // You can restrict to your front‐end origin here.
		handlers.AllowedMethods([]string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		}),
		handlers.AllowedHeaders([]string{
			"Authorization",
			"Content-Type",
			"X-Requested-With",
		}),
	)

	// Create the HTTP server on port 7080 (to match your curl)
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      corsHandler(r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("Server starting on :8080")
	log.Fatal(srv.ListenAndServe())
}
