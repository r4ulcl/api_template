package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
)

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
