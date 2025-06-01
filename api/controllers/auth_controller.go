// file: controllers/auth_controller.go

package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"

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
func (ac *AuthController) RegisterUser(user *models.User) (*models.User, error) {
	// Trim & validate
	user.Username = strings.TrimSpace(user.Username)
	if user.Username == "" || user.Password == "" {
		return nil, errInvalidInput
	}

	// Hash the plaintext password
	hashed, err := utils.HashPassword(user.Password)
	if err != nil {
		return nil, err
	}
	user.Password = hashed

	// Insert into DB
	if err := ac.BC.CreateOrUpdateRecord(user, true); err != nil {
		return nil, err
	}

	// Clear out Password before returning
	user.Password = ""
	return user, nil
}

// Register is the HTTP handler that leverages RegisterUser()
// to perform the actual user registration logic.
func (ac *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var userInput models.User
	if err := json.NewDecoder(r.Body).Decode(&userInput); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input JSON"})
		return
	}

	createdUser, err := ac.RegisterUser(&userInput)
	switch err {
	case nil:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createdUser)
	case errInvalidInput:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Username and password cannot be empty"})
	case errUserAlreadyExists:
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User already exists"})
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
	}
}

// Login handles both user login (POST), token renewal (PUT), and fetching user info (GET).
func (ac *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		ac.handleLogin(w, r)
	case http.MethodGet:
		ac.handleGetUserInfo(w, r)
	case http.MethodPut:
		ac.handleRenewToken(w, r)
	default:
		w.Header().Set("Allow", "POST, GET, PUT")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogin processes POST /login: authenticates and returns a JWT token.
func (ac *AuthController) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid input"})
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Username and password cannot be empty"})
		return
	}

	// Fetch the user by primary key (username)
	var user models.User
	if err := ac.BC.GetRecordsByID(&user, input.Username); err != nil {

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
	tokenString, err := utils.GenerateJWT(user.Username, string(user.Role), ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate token"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

// handleGetUserInfo processes GET /login: validates the JWT and returns user info.
func (ac *AuthController) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	// Expect Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing Authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid Authorization header format"})
		return
	}
	tokenString := parts[1]

	// Parse and validate token
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(ac.Secret), nil
	})
	if err != nil || !token.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid or expired token"})
		return
	}

	// Extract username from claims
	usernameInterface, ok := claims["username"]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token missing username claim"})
		return
	}
	username, ok := usernameInterface.(string)
	if !ok || username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username claim"})
		return
	}

	// Fetch user record
	var user models.User
	if err := ac.BC.GetRecordsByID(&user, username); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		return
	}

	// Clear sensitive fields
	user.Password = ""

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
}

// handleRenewToken processes PUT /login: validates the JWT and issues a new one.
func (ac *AuthController) handleRenewToken(w http.ResponseWriter, r *http.Request) {
	// Expect Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing Authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid Authorization header format"})
		return
	}
	oldTokenString := parts[1]

	// Parse and validate old token
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(oldTokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(ac.Secret), nil
	})
	if err != nil || !token.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid or expired token"})
		return
	}

	// Extract claims
	usernameInterface, userOK := claims["username"]
	roleInterface, roleOK := claims["role"]
	if !userOK || !roleOK {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token missing required claims"})
		return
	}
	username, ok1 := usernameInterface.(string)
	role, ok2 := roleInterface.(string)
	if !ok1 || !ok2 || username == "" || role == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid claims in token"})
		return
	}

	// Optionally: ensure token isn't too old to renew
	if iatRaw, exists := claims["iat"]; exists {
		if iatFloat, ok := iatRaw.(float64); ok {
			issuedAt := time.Unix(int64(iatFloat), 0)
			if time.Since(issuedAt) > 7*24*time.Hour {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token too old to renew"})
				return
			}
		}
	}

	// Generate a new token
	newTokenString, err := utils.GenerateJWT(username, role, ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate new token"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": newTokenString})
}
