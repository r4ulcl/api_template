package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/models"
	"github.com/r4ulcl/api_template/utils"
)

// AuthController handles authentication-related requests, including user registration and login.
type AuthController struct {
	Secret string // Secret key for signing JWT tokens
}

// Register handles user registration by accepting the user's input, validating it, and storing it in the database.
func (ac *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	// Set response header to indicate JSON content
	w.Header().Set("Content-Type", "application/json")

	// Create an empty user model to store the incoming data
	var user models.User
	// Decode incoming JSON body into the user model
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		// If decoding fails, return a Bad Request status with an error message
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input"})
		return
	}

	// Set default role to "user" if not provided, or "admin" if explicitly set
	admin := false
	if user.Role == "admin" {
		admin = true
	}

	// Call the database function to create the user
	err := database.CreateUser(user.Username, user.Password, admin)
	if err != nil {
		// If there's an error creating the user, return an Internal Server Error status with the error message
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// Return a Created status with the user data
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

// Login handles user login and token generation.
func (ac *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	// Set response header to indicate JSON content
	w.Header().Set("Content-Type", "application/json")

	// Create a structure to hold the login input
	var input struct {
		Username string `json:"username"` // Username of the user
		Password string `json:"password"` // Password of the user
	}

	// Decode the incoming JSON body into the input structure
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// If decoding fails, return a Bad Request status with an error message
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input"})
		return
	}

	// Create a user object to fetch from the database
	var user models.User
	// Query the database for the user by username
	if err := database.DB.Where("username = ?", input.Username).First(&user).Error; err != nil {
		// If the user is not found, return Unauthorized status with an error message
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})
		return
	}

	// Check if the provided password matches the stored password hash
	if err := utils.CheckPassword(user.Password, input.Password); err != nil {
		// If password verification fails, return Unauthorized status with an error message
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})
		return
	}

	// Generate a JWT token for the user after successful login
	token, err := utils.GenerateJWT(user.Username, string(user.Role), ac.Secret)
	if err != nil {
		// If token generation fails, return Internal Server Error status with an error message
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate token"})
		return
	}

	// Return a successful response with the generated token
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}
