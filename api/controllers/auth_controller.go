package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
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
	// Trim and validate
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

	if user.CreatedBy == "" {
		user.CreatedBy = "SYSTEM"
	}
	if user.EditedBy == "" {
		user.EditedBy = "SYSTEM"
	}

	// Insert into DB
	if err := ac.BC.CreateOrUpdateRecord(user, false); err != nil {
		return nil, err
	}

	// Clear out Password before returning
	user.Password = ""
	return user, nil
}

// forceCreatedBy sets CreatedBy if the field exists and we have a non-empty userID.
func forceCreatedBy(model interface{}, userID string) {
	if userID == "" {
		return
	}
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if !v.IsValid() {
		return
	}
	if f := v.FieldByName("CreatedBy"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(userID)
	}
}

// Register is the HTTP handler that leverages RegisterUser()
// to perform the actual user registration logic.
//
// @Summary     Register a new user account
// @Description Accepts JSON input with username and password to create a new user.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       user  body      models.User            true  "User registration data"
// @Success     201   {object}  models.User            "User successfully registered"
// @Failure     400   {object}  models.ErrorResponse   "Invalid input JSON or missing fields"
// @Failure     409   {object}  models.ErrorResponse   "User already exists"
// @Failure     500   {object}  models.ErrorResponse   "Internal server error"
// @Router      /register [post]
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
		// audit bad request
		logAudit(&Controller{BC: ac.BC}, r, "", "create", "users", "", http.StatusBadRequest, nil)
		return
	}

	// Pull caller context if present to populate audit fields
	_, callerUserID := ownOnlyAndUserID(r)

	// Set audit fields on create and lock ownership to the caller when available
	setAuditOnCreate(&userInput, callerUserID)
	forceCreatedBy(&userInput, callerUserID)

	createdUser, err := ac.RegisterUser(&userInput)
	switch err {
	case nil:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createdUser)
		// audit success
		logAudit(&Controller{BC: ac.BC}, r, callerUserID, "create", "users", createdUser.Username, http.StatusCreated, &auditChange{
			After: createdUser,
		})
	case errInvalidInput:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Username and password cannot be empty"})
		logAudit(&Controller{BC: ac.BC}, r, callerUserID, "create", "users", "", http.StatusBadRequest, nil)
	case errUserAlreadyExists:
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User already exists"})
		logAudit(&Controller{BC: ac.BC}, r, callerUserID, "create", "users", userInput.Username, http.StatusConflict, nil)
	default:
		// Surface duplicate constraint errors consistently as conflicts
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			logAudit(&Controller{BC: ac.BC}, r, callerUserID, "create", "users", userInput.Username, http.StatusConflict, nil)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(&Controller{BC: ac.BC}, r, callerUserID, "create", "users", userInput.Username, http.StatusInternalServerError, nil)
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
		w.Header().Set("Allow", "POST, PUT")
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
		logAudit(&Controller{BC: ac.BC}, r, "", "login", "auth", "", http.StatusBadRequest, nil)
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Username and password cannot be empty"})
		logAudit(&Controller{BC: ac.BC}, r, input.Username, "login", "auth", input.Username, http.StatusBadRequest, nil)
		return
	}

	// Fetch the user by primary key (username)
	var user models.User
	if err := ac.BC.GetRecordsByIDWithSensitive(&user, input.Username); err != nil {

		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})
		logAudit(&Controller{BC: ac.BC}, r, input.Username, "login", "auth", input.Username, http.StatusUnauthorized, nil)
		return
	}

	// Check password
	if err := utils.CheckPassword(user.Password, input.Password); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid username or password"})
		logAudit(&Controller{BC: ac.BC}, r, input.Username, "login", "auth", input.Username, http.StatusUnauthorized, nil)
		return
	}

	// Generate JWT token
	tokenString, err := utils.GenerateJWT(user.Username, string(user.Role), ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate token"})
		logAudit(&Controller{BC: ac.BC}, r, user.Username, "login", "auth", user.Username, http.StatusInternalServerError, nil)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
	// success audit
	logAudit(&Controller{BC: ac.BC}, r, user.Username, "login", "auth", user.Username, http.StatusOK, nil)
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
		logAudit(&Controller{BC: ac.BC}, r, username, "renew_token", "auth", username, http.StatusUnauthorized, nil)
		return
	}

	// Generate a new token
	newTokenString, err := utils.GenerateJWT(username, role, ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate new token"})
		logAudit(&Controller{BC: ac.BC}, r, username, "renew_token", "auth", username, http.StatusInternalServerError, nil)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": newTokenString})
	logAudit(&Controller{BC: ac.BC}, r, username, "renew_token", "auth", username, http.StatusOK, nil)
}
