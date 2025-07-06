package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/r4ulcl/api_template/api/middlewares"
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

func (ac *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		ac.handleLogin(w, r)
	case http.MethodPut:
		ac.handleRenewToken(w, r)
	default:
		w.Header().Set("Allow", "POST, GET, PUT")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogin processes POST /login: authenticates and returns a JWT token.
// Login handles both user login (POST), token renewal (PUT), and fetching user info (GET).
// @Summary     Authenticate a user and issue a JWT (POST)
// @Description Accepts JSON credentials (username + password) and returns a token if valid.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       credentials  body      models.LoginRequest   true  "Username and password"
// @Success     200          {object}  models.JWTResponse    "JWT token returned"
// @Failure     400          {object}  models.ErrorResponse  "Missing or invalid fields"
// @Failure     401          {object}  models.ErrorResponse  "Unauthorized (invalid credentials)"
// @Failure     500          {object}  models.ErrorResponse  "Internal server error"
// @Router      /login [post]
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

// @Summary     Authenticate user via query params (GET)
// @Description Accepts username and password as query parameters and returns a token if valid.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Success     200       {object}  models.JWTResponse    "JWT token returned"
// @Failure     400       {object}  models.ErrorResponse  "Missing or invalid fields"
// @Failure     401       {object}  models.ErrorResponse  "Unauthorized (invalid credentials)"
// @Failure     500       {object}  models.ErrorResponse  "Internal server error"
// @Router      /login [get]
func (ac *AuthController) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	// Retrieve user ID from context (set by AuthMiddleware)
	userIDVal := r.Context().Value(middlewares.ContextUserID)
	username, ok := userIDVal.(string)
	if !ok || username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized: no user in context"})
		return
	}

	// Fetch user record by username (userID)
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

// @Summary     Renew token via PUT
// @Description Accepts JWT using PUT. Returns a new JWT if valid.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Success     200          {object}  models.JWTResponse    "JWT token returned"
// @Failure     400          {object}  models.ErrorResponse  "Missing or invalid fields"
// @Failure     401          {object}  models.ErrorResponse  "Unauthorized (invalid credentials)"
// @Failure     500          {object}  models.ErrorResponse  "Internal server error"
// @Router      /login [put]
func (ac *AuthController) handleRenewToken(w http.ResponseWriter, r *http.Request) {
	// Retrieve username and role from context (set by AuthMiddleware)
	userIDVal := r.Context().Value(middlewares.ContextUserID)
	roleVal := r.Context().Value(middlewares.ContextRole)

	username, ok1 := userIDVal.(string)
	role, ok2 := roleVal.(string)
	if !ok1 || username == "" || !ok2 || role == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: missing user or role in context"})
		return
	}

	// Generate a new token
	newTokenString, err := utils.GenerateJWT(username, role, ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate new token"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": newTokenString})
}

// Me handles GET /me (fetch profile) and POST /me (update profile)
// @Summary     Get or update current user's profile
// @Description GET returns the authenticated user's info; POST updates fields like email or password.
// @Tags        user, auth
// @Accept      json
// @Produce     json
// @Success     200   {object} models.User             "User info returned or updated"
// @Failure     400   {object} models.ErrorResponse    "Invalid input JSON"
// @Failure     401   {object} models.ErrorResponse    "Unauthorized: missing or invalid token"
// @Failure     404   {object} models.ErrorResponse    "User not found"
// @Failure     500   {object} models.ErrorResponse    "Internal server error"
// @Router      /me [get]
func (ac *AuthController) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		ac.handleGetUserInfo(w, r)
	case http.MethodPost:
		ac.handleUpdateUserInfo(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUpdateUserInfo processes POST /me: update own info (e.g. password, email, etc.)
// @Summary     Update current user's profile
// @Description Allows the authenticated user to change email and/or password. To change password, both current and new passwords are required.
// @Tags        user, auth
// @Accept      json
// @Produce     json
// @Param       update body     models.UpdateUser true "Fields to update"
// @Success     200    {object} models.User             "Updated user info"
// @Failure     400    {object} models.ErrorResponse    "Bad request (invalid/missing fields)"
// @Failure     401    {object} models.ErrorResponse    "Unauthorized (invalid current password or token)"
// @Failure     404    {object} models.ErrorResponse    "User not found"
// @Failure     500    {object} models.ErrorResponse    "Internal server error"
// @Router      /me [post]
func (ac *AuthController) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	// 1) Get current user & role from context
	uidVal := r.Context().Value(middlewares.ContextUserID)
	rlVal := r.Context().Value(middlewares.ContextRole)
	currentUsername, _ := uidVal.(string)
	currentRole, _ := rlVal.(string)
	if currentUsername == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized"})
		return
	}

	// 2) Load their record
	var user models.User
	if err := ac.BC.GetRecordsByID(&user, currentUsername); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		return
	}

	// 3) Decode into a generic map so we can inspect all keys
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON"})
		return
	}

	// 4) Define which roles get “power” access
	powerRoles := map[string]bool{
		string(models.AdminRole): true,
		// add other elevated roles here...
	}

	// 5) Build the two whitelists
	regularAllowed := map[string]bool{
		"username":     true,
		"password":     true, // for verifying current password
		"new_password": true,
	}
	adminAllowed := map[string]bool{
		"username":     true,
		"role":         true,
		"password":     true,
		"new_password": true,
		// add any other JSON-exposed User fields (e.g. email) here
	}

	// 6) Pick the right whitelist
	var allowed map[string]bool
	if powerRoles[currentRole] {
		allowed = adminAllowed
	} else {
		allowed = regularAllowed
	}

	// 7) Reject any disallowed fields
	for key := range payload {
		if !allowed[key] {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: fmt.Sprintf("cannot update field %q", key),
			})
			return
		}
	}

	// 8) Apply changes

	// — Username
	if raw, ok := payload["username"]; ok {
		if newU, ok2 := raw.(string); ok2 && newU != "" && newU != user.Username {
			user.Username = strings.TrimSpace(newU)
		}
	}

	// — Role (only if in adminAllowed, i.e. powerRoles)
	if raw, ok := payload["role"]; ok && allowed["role"] {
		if r2, ok2 := raw.(string); ok2 {
			user.Role = models.Role(r2)
		}
	}

	// — Password change
	if rawNew, ok := payload["new_password"]; ok {
		newP, _ := rawNew.(string)
		if newP != "" {
			currRaw, hasCurr := payload["password"]
			currP, _ := currRaw.(string)
			if !hasCurr || currP == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password required"})
				return
			}
			if err := utils.CheckPassword(user.Password, currP); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password incorrect"})
				return
			}
			hashed, err := utils.HashPassword(newP)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "failed to hash new password"})
				return
			}
			user.Password = hashed
		}
	}

	// 9) Persist
	if err := ac.BC.CreateOrUpdateRecord(&user, false); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 10) Return sanitized user
	user.Password = ""
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
}
