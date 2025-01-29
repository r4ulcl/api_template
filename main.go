package main

import (
	"log"
	"net/http"

	"github.com/r4ulcl/api_template/api/controllers"
	"github.com/r4ulcl/api_template/api/routes"
	"github.com/r4ulcl/api_template/database"
	_ "github.com/r4ulcl/api_template/docs"
	"github.com/r4ulcl/api_template/utils"
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

	// Connect to the database using loaded configuration
	database.ConnectDB(cfg)

	// Create default admin user
	username := "admin"
	err := database.CreateUser(username, cfg.AdminPassword, true)
	if err != nil {
		log.Println("Error creating default user")
	}

	// Initialize controllers
	authController := &controllers.AuthController{Secret: cfg.JWTSecret}
	baseController := &database.BaseController{DB: database.DB}
	controller := &controllers.Controller{BC: baseController}

	// Setup the router
	r := routes.SetupRouter(controller, authController, cfg.JWTSecret)

	// Start the HTTP server
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
