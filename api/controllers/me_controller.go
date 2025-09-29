package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// Me handles GET /me and PATCH /me
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
// @Router      /me [PATCH]
// @Router      /me/api-key [post]
func (ac *AuthController) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		ac.handleGetUserInfo(w, r)
	case http.MethodPatch:
		ac.handleUpdateUserInfo(w, r)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUpdateUserInfo processes PATH /me: update own info (e.g. password, email, etc.)
// @Summary     Update current user's profile
// @Description Allows the authenticated user to change email and/or password. To change password, both current and new passwords are required.
// @Tags        user
// @Accept      json
// @Produce     json
// @Param       update body     models.UpdateUser true "Fields to update"
// @Success     200    {object} models.User             "Updated user info"
// @Failure     400    {object} models.ErrorResponse    "Bad request (invalid/missing fields)"
// @Failure     401    {object} models.ErrorResponse    "Unauthorized (invalid current password or token)"
// @Failure     404    {object} models.ErrorResponse    "User not found"
// @Failure     500    {object} models.ErrorResponse    "Internal server error"
// @Router      /me [PATH]
// handleUpdateUserInfo processes PATH /me: update own info (e.g. password, email, etc.)
func (ac *AuthController) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	// 1) Auth context
	uidVal := r.Context().Value(middlewares.ContextUserID)
	rlVal := r.Context().Value(middlewares.ContextRole)
	currentUsername, _ := uidVal.(string)
	currentRoleStr, _ := rlVal.(string)
	if strings.TrimSpace(currentUsername) == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized"})
		return
	}
	currentRole := models.Role(currentRoleStr)

	// 2) Load current user's record
	var user models.User
	if err := ac.BC.GetRecordsByID(&user, currentUsername); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		return
	}

	// 3) Decode body
	const maxBody = 1 << 20 // 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON"})
		return
	}
	bodyBytes, _ := json.Marshal(payload)
	var req models.UpdateUser
	_ = json.Unmarshal(bodyBytes, &req)

	// 4) Disallow username and audit fields for everyone
	disallowedAlways := map[string]bool{
		"username":    true,
		"created_at":  true,
		"last_update": true,
		"created_by":  true,
		"edited_by":   true,
	}

	// 5) Field-level gate
	allowed := models.UpdateUserWhitelist[currentRole]

	for key := range payload {
		if disallowedAlways[key] {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: fmt.Sprintf("cannot update field %q via this endpoint", key),
			})
			return
		}
		// Admin can edit anything that is not in disallowedAlways
		if currentRole == models.AdminRole {
			continue
		}
		// Regular users can only touch password change inputs
		if !allowed[key] {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: fmt.Sprintf("cannot update field %q", key),
			})
			return
		}
	}

	// 6) Apply editable fields
	if currentRole == models.AdminRole {
		// admin path
		if _, ok := payload["email"]; ok {
			user.Email = strings.TrimSpace(req.Email)
		}
		if _, ok := payload["email_verified"]; ok {
			user.EmailVerified = req.EmailVerified
		}
		if _, ok := payload["role"]; ok {
			roleStr := strings.TrimSpace(string(req.Role))
			switch models.Role(roleStr) {
			case models.AdminRole, models.UserRole:
				user.Role = models.Role(roleStr)
			default:
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "invalid role value"})
				return
			}
		}
	} else {
		// user path
		if _, ok := payload["email"]; ok {
			newEmail := strings.TrimSpace(req.Email)
			if newEmail == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "email cannot be empty"})
				return
			}
			// Only flip verification if the email actually changes
			if newEmail != user.Email {
				user.Email = newEmail
				user.EmailVerified = false
			}
		}
	}

	// 7) Password change
	if _, ok := payload["new_password"]; ok && strings.TrimSpace(req.NewPassword) != "" {
		if currentRole == models.AdminRole {
			hashed, err := utils.HashPassword(req.NewPassword)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "failed to hash new password"})
				return
			}
			user.Password = hashed
		} else {
			currRaw, hasCurr := payload["password"]
			currP, _ := currRaw.(string)
			if !hasCurr || strings.TrimSpace(currP) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password required"})
				return
			}
			if err := utils.CheckPassword(user.Password, currP); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password incorrect"})
				return
			}
			hashed, err := utils.HashPassword(req.NewPassword)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "failed to hash new password"})
				return
			}
			user.Password = hashed
		}
	}

	// 8) Persist with UPDATE
	user.Username = currentUsername
	user.EditedBy = currentUsername
	user.LastUpdate = time.Now()

	updates := map[string]interface{}{
		"edited_by":   user.EditedBy,
		"last_update": user.LastUpdate,
	}

	if currentRole == models.AdminRole {
		if _, ok := payload["email"]; ok {
			updates["email"] = user.Email
		}
		if _, ok := payload["email_verified"]; ok {
			updates["email_verified"] = user.EmailVerified
		}
		if _, ok := payload["role"]; ok {
			updates["role"] = user.Role
		}
	} else {
		if _, ok := payload["email"]; ok {
			updates["email"] = user.Email
			updates["email_verified"] = user.EmailVerified // forced false if changed
		}
	}

	if _, ok := payload["new_password"]; ok && strings.TrimSpace(req.NewPassword) != "" {
		updates["password"] = user.Password
	}

	if len(updates) == 2 {
		// only audit changed
		w.WriteHeader(http.StatusOK)
		user.Password = ""
		_ = json.NewEncoder(w).Encode(user)
		return
	}

	if err := ac.BC.DB.Model(&models.User{}).
		Where("username = ?", user.Username).
		Updates(updates).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// return sanitized
	user.Password = ""
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
}

// @Summary     Authenticate user via query params (GET)
// @Description Get userinformation
// @Tags        user
// @Accept      json
// @Produce     json
// @Success     200       {object}  models.JWTResponse    "JWT token returned"
// @Failure     400       {object}  models.ErrorResponse  "Missing or invalid fields"
// @Failure     401       {object}  models.ErrorResponse  "Unauthorized (invalid credentials)"
// @Failure     500       {object}  models.ErrorResponse  "Internal server error"
// @Router      /me [get]
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

// handleGenerateAPIKey processes POST /me/api-key: generate a new permanent API key
// @Summary     Generate API key
// @Description Allows the authenticated user to generate a new API key without expiration for use in scripts or integrations.
// @Tags        user, auth
// @Accept      json
// @Produce     json
// @Success     200    {object} map[string]string       "API key successfully created"
// @Failure     400    {object} models.ErrorResponse    "Bad request (invalid input)"
// @Failure     401    {object} models.ErrorResponse    "Unauthorized (invalid or missing token)"
// @Failure     404    {object} models.ErrorResponse    "User not found"
// @Failure     409    {object} models.ErrorResponse    "Duplicate API key (unexpected conflict)"
// @Failure     500    {object} models.ErrorResponse    "Internal server error"
// @Router      /me/api-key [post]
func (ac *AuthController) GenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	uidVal := r.Context().Value(middlewares.ContextUserID)
	rlVal := r.Context().Value(middlewares.ContextRole)
	username, _ := uidVal.(string)
	role, _ := rlVal.(string)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized"})
		return
	}

	token, err := utils.GenerateJWTNoExpiry(map[string]interface{}{
		"username": username,
		"role":     role,
	}, ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate API key"})
		return
	}

	err = ac.createAPIKey(username, token)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"api_key": token})
	case errors.Is(err, gorm.ErrRecordNotFound):
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
	case strings.Contains(err.Error(), "Duplicate entry"):
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Duplicate API key"})
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
	}
}

// createAPIKey inserts a new API key for a given user.
func (ac *AuthController) createAPIKey(username, token string) error {
	// Ensure user exists
	var user models.User
	if err := ac.BC.DB.First(&user, "username = ?", username).Error; err != nil {
		return err
	}

	apiKey := models.APIKey{
		Token:    token,
		Username: username,
		Enabled:  true,
	}
	return ac.BC.DB.Create(&apiKey).Error
}

// handleDeleteAPIKey processes DELETE /me/api-key/{apiKey}
// @Summary     Revoke API key
// @Description Disables a specific API key for the authenticated user so it can no longer be used.
// @Tags        user, auth
// @Accept      json
// @Produce     json
// @Param       apiKey path string true "API key to revoke"
// @Success     200    {object} map[string]bool         "revoked: true"
// @Failure     400    {object} models.ErrorResponse    "Bad request (missing apiKey)"
// @Failure     401    {object} models.ErrorResponse    "Unauthorized (invalid or missing token)"
// @Failure     404    {object} models.ErrorResponse    "API key not found"
// @Failure     500    {object} models.ErrorResponse    "Internal server error"
// @Router      /me/api-key/{apiKey} [delete]
func (ac *AuthController) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	uidVal := r.Context().Value(middlewares.ContextUserID)
	username, _ := uidVal.(string)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized"})
		return
	}

	apiKey := mux.Vars(r)["apiKey"]
	if apiKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing apiKey in path"})
		return
	}

	err := ac.disableAPIKey(username, apiKey)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": true})
	case errors.Is(err, gorm.ErrRecordNotFound):
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "API key not found"})
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
	}
}

// disableAPIKey disables (revokes) a user's API key.
func (ac *AuthController) disableAPIKey(username, token string) error {
	var rec models.APIKey
	if err := ac.BC.DB.First(&rec, "token = ? AND username = ?", token, username).Error; err != nil {
		return err
	}
	return ac.BC.DB.Model(&rec).Update("enabled", false).Error
}
