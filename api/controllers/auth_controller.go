// file: controllers/auth_controller.go

package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
)

// AuthController Struct for secret and database.BaseController.
type AuthController struct {
	Secret string
	BC     *database.BaseController
}

var (
	errInvalidInput      = errors.New("invalid input")
	errUserAlreadyExists = errors.New("user already exists")
)

// RegisterUser contains the core logic for creating a new user in the DB.
// It checks for existing usernames, hashes the password, and inserts into the DB.
//
// Return values:
//  1. The newly created user (without the raw password).
//  2. An error if something went wrong.
func (ac *AuthController) RegisterUser(user models.User) (models.User, error) {
	// Basic validation
	if user.Username == "" || user.Password == "" {
		return user, errInvalidInput // you can define a sentinel error
	}

	// Hash the password
	hashedPass, err := utils.HashPassword(user.Password)
	if err != nil {
		return user, err
	}

	user.Password = hashedPass

	// Assign role
	if user.Role == "admin" {
		user.Role = models.AdminRole
	} else {
		user.Role = models.UserRole
	}

	// Create the user in DB
	if err := ac.BC.CreateOrUpdateRecord(&user, true); err != nil {
		return user, err
	}

	// Return the newly-created user (password is already hashed)
	return user, nil
}

// Register is the HTTP handler that leverages RegisterUser()
// to perform the actual user registration logic.
func (ac *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Decode user input
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	createdUser, err := ac.RegisterUser(user)
	switch err {
	case nil:
		// Successfully created
		w.WriteHeader(http.StatusCreated)
		// NOTE: You may want to omit the password from the response here
		_ = json.NewEncoder(w).Encode(createdUser)
	case errInvalidInput:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Username and password cannot be empty"})
	case errUserAlreadyExists:
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User already exists"})
	default:
		// Any other error
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
	}
}

// Login existing user.
func (ac *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input"})

		return
	}

	// Fetch the user by primary key (username)
	var user models.User

	err := ac.BC.GetRecordsByID(&user, input.Username)
	if err != nil {
		// Either user not found or other DB error
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})

		return
	}

	// Check password
	if err := utils.CheckPassword(user.Password, input.Password); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})

		return
	}

	// Generate JWT token
	token, err := utils.GenerateJWT(user.Username, string(user.Role), ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate token"})

		return
	}

	// Return token
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}
