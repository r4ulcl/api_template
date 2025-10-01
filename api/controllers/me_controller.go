package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// helper to mask sensitive user fields for audit logs
func sanitizeUserForAudit(u models.User) models.User {
	u.Password = ""
	u.TotpSecret = ""
	// API keys are not embedded in User, so nothing else to redact here
	return u
}

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

// handleUpdateUserInfo processes PATCH /me: update own info (e.g. password, email, etc.)
// @Summary     Update current user's profile
// @Description Allows the authenticated user to change email and/or password. To change password, both current and new passwords are required. Mint a TOTP secret with `totp_reset_secret` and include `totp_code` when enabling or disabling MFA.
// @Tags        user
// @Accept      json
// @Produce     json
// @Param       update body     models.UpdateUser true "Fields to update"
// @Success     200    {object} models.User             "Updated user info"
// @Failure     400    {object} models.ErrorResponse    "Bad request (invalid/missing fields)"
// @Failure     401    {object} models.ErrorResponse    "Unauthorized (invalid current password or token)"
// @Failure     404    {object} models.ErrorResponse    "User not found"
// @Failure     500    {object} models.ErrorResponse    "Internal server error"
// @Router      /me [PATCH]
// handleUpdateUserInfo processes PATCH /me: update own info (e.g. password, email, etc.)
func (ac *AuthController) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	// 1) Auth context
	uidVal := r.Context().Value(middlewares.ContextUserID)
	rlVal := r.Context().Value(middlewares.ContextRole)
	currentUsername, _ := uidVal.(string)
	currentRoleStr, _ := rlVal.(string)
	if strings.TrimSpace(currentUsername) == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unauthorized"})
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusUnauthorized, nil)
		return
	}
	currentRole := models.Role(currentRoleStr)

	// 2) Load current user's record
	var user models.User
	if err := ac.BC.GetRecordsByIDWithSensitive(&user, currentUsername); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusNotFound, nil)
		return
	}
	user.TotpSecret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(user.TotpSecret), " ", ""))
	if user.TotpSecret == "" {
		user.TotpEnabled = false
	}
	before := sanitizeUserForAudit(user)

	// 3) Decode body
	const maxBody = 1 << 20 // 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON"})
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
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
	allowed, ok := models.UpdateUserWhitelist[currentRole]
	if !ok {
		allowed = models.UpdateUserWhitelist[models.DefaultRoleWhitelist]
	}

	for key := range payload {
		if disallowedAlways[key] {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: fmt.Sprintf("cannot update field %q via this endpoint", key),
			})
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
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
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
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
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
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
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
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
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusInternalServerError, &auditChange{Before: before})
				return
			}
			user.Password = hashed
		} else {
			currRaw, hasCurr := payload["password"]
			currP, _ := currRaw.(string)
			if !hasCurr || strings.TrimSpace(currP) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password required"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			if err := utils.CheckPassword(user.Password, currP); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "current password incorrect"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusUnauthorized, &auditChange{Before: before})
				return
			}
			hashed, err := utils.HashPassword(req.NewPassword)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "failed to hash new password"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusInternalServerError, &auditChange{Before: before})
				return
			}
			user.Password = hashed
		}
	}

	// 7a) TOTP configuration
	secretChanged := false
	var newTotpSecret string

	sanitizeTotpCode := func(raw string) string {
		raw = strings.TrimSpace(raw)
		var builder strings.Builder
		for _, r := range raw {
			if unicode.IsDigit(r) {
				builder.WriteRune(r)
			}
		}
		return builder.String()
	}

	if _, ok := payload["totp_secret"]; ok {
		currentSecret := user.TotpSecret
		sanitized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(req.TotpSecret), " ", ""))
		if sanitized != currentSecret {
			secretChanged = true
		}
		user.TotpSecret = sanitized
		if sanitized == "" {
			user.TotpEnabled = false
		} else if sanitized != currentSecret {
			user.TotpEnabled = false
		}
	}

	resetSecret := false
	if raw, ok := payload["totp_reset_secret"]; ok {
		if flag, ok := raw.(bool); ok && flag {
			resetSecret = true
		}
	}

	if resetSecret {
		if req.TotpEnabled != nil && !*req.TotpEnabled {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "cannot reset TOTP secret while disabling"})
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
			return
		}
		if user.TotpEnabled {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "disable TOTP before generating a new secret"})
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
			return
		}
		secret, err := utils.GenerateTOTPSecret()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "failed to generate TOTP secret"})
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusInternalServerError, &auditChange{Before: before})
			return
		}
		user.TotpSecret = secret
		user.TotpEnabled = false
		secretChanged = true
		newTotpSecret = secret
	}

	if _, ok := payload["totp_enabled"]; ok {
		if req.TotpEnabled == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "totp_enabled requires a boolean value"})
			logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
			return
		}
		if *req.TotpEnabled {
			if strings.TrimSpace(user.TotpSecret) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "generate a TOTP secret before enabling"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			code := sanitizeTotpCode(req.TotpCode)
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "totp_code required when enabling"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			if !utils.ValidateTOTP(user.TotpSecret, code) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "invalid TOTP code"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			user.TotpEnabled = true
		} else {
			if strings.TrimSpace(user.TotpSecret) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "TOTP is not currently configured"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			code := sanitizeTotpCode(req.TotpCode)
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "totp_code required when disabling"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			if !utils.ValidateTOTP(user.TotpSecret, code) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "invalid TOTP code"})
				logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
				return
			}
			user.TotpEnabled = false
		}
	}

	if user.TotpEnabled && strings.TrimSpace(user.TotpSecret) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "TOTP secret must be set before enabling"})
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusBadRequest, &auditChange{Before: before})
		return
	}

	// 8) Persist with UPDATE
	user.Username = currentUsername
	user.EditedBy = currentUsername
	user.LastUpdate = time.Now()

	updates := map[string]interface{}{
		"edited_by":   user.EditedBy,
		"last_update": user.LastUpdate,
	}

	if secretChanged {
		updates["totp_secret"] = user.TotpSecret
		updates["totp_enabled"] = user.TotpEnabled
	} else if _, ok := payload["totp_enabled"]; ok {
		updates["totp_enabled"] = user.TotpEnabled
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
		// nothing changed beyond audit fields
		w.WriteHeader(http.StatusOK)
		user.Password = ""
		user.TotpSecret = ""
		resp := struct {
			models.User
			NewTotpSecret string `json:"new_totp_secret,omitempty"`
		}{
			User: user,
		}
		if newTotpSecret != "" {
			resp.NewTotpSecret = newTotpSecret
		}
		_ = json.NewEncoder(w).Encode(resp)
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusOK, &auditChange{
			Before: before,
			After:  sanitizeUserForAudit(user),
		})
		return
	}

	if err := ac.BC.DB.Model(&models.User{}).
		Where("username = ?", user.Username).
		Updates(updates).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusInternalServerError, &auditChange{Before: before})
		return
	}

	// return sanitized
	user.Password = ""
	user.TotpSecret = ""
	w.WriteHeader(http.StatusOK)
	resp := struct {
		models.User
		NewTotpSecret string `json:"new_totp_secret,omitempty"`
	}{
		User: user,
	}
	if newTotpSecret != "" {
		resp.NewTotpSecret = newTotpSecret
	}
	_ = json.NewEncoder(w).Encode(resp)
	logAudit(&Controller{BC: ac.BC}, r, currentUsername, "me_update", "users", currentUsername, http.StatusOK, &auditChange{
		Before: before,
		After:  sanitizeUserForAudit(user),
	})
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
		logAudit(&Controller{BC: ac.BC}, r, "", "read", "users", "", http.StatusUnauthorized, nil)
		return
	}

	// Fetch user record by username (userID)
	var user models.User
	if err := ac.BC.GetRecordsByID(&user, username); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		logAudit(&Controller{BC: ac.BC}, r, username, "read", "users", username, http.StatusNotFound, nil)
		return
	}

	// Clear sensitive fields
	user.Password = ""

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
	logAudit(&Controller{BC: ac.BC}, r, username, "read", "users", username, http.StatusOK, nil)
}

// handleGenerateAPIKey processes POST /me/api-key: generate a new API key.
// @Summary     Generate API key
// @Description Allows the authenticated user to mint a new API key after providing a description and optional expiration timestamp.
// @Tags        user, auth
// @Accept      json
// @Produce     json
// @Param       payload body     models.CreateAPIKeyRequest true "API key details"
// @Success     201    {object} models.APIKeyResponse     "API key metadata (token omitted)"
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
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusUnauthorized, nil)
		return
	}

	const maxBody = 1 << 20 // 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	defer func() { _ = r.Body.Close() }()

	var payload models.CreateAPIKeyRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "request body is required"})
			logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusBadRequest, nil)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "invalid JSON payload"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusBadRequest, nil)
		return
	}

	if err := decoder.Decode(new(struct{})); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "unexpected data after JSON payload"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusBadRequest, nil)
		return
	}

	description := strings.TrimSpace(payload.Description)
	if description == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "description is required"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusBadRequest, nil)
		return
	}

	var expiracy *time.Time
	if payload.Expiracy != nil {
		exp := payload.Expiracy.UTC()
		if exp.Before(time.Now().UTC()) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "expiracy must be in the future"})
			logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusBadRequest, nil)
			return
		}
		expiracy = &exp
	}

	token, err := utils.GenerateJWTNoExpiry(map[string]interface{}{
		"username": username,
		"role":     role,
	}, ac.Secret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Failed to generate API key"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusInternalServerError, nil)
		return
	}

	apiKey, err := ac.createAPIKey(username, token, description, expiracy)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
		resp := sanitizeAPIKeyForResponse(apiKey)
		resp.APIKey = token
		_ = json.NewEncoder(w).Encode(resp)
		// only store masked token in audit logs
		masked := ""
		if len(token) > 12 {
			masked = token[:8] + "...(masked)"
		}
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", masked, http.StatusCreated, nil)
	case errors.Is(err, gorm.ErrRecordNotFound):
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "User not found"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusNotFound, nil)
	case strings.Contains(err.Error(), "Duplicate entry"):
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Duplicate API key"})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusConflict, nil)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(&Controller{BC: ac.BC}, r, username, "create_api_key", "api_keys", "", http.StatusInternalServerError, nil)
	}
}

// createAPIKey inserts a new API key for a given user.
func (ac *AuthController) createAPIKey(username, token, description string, expiracy *time.Time) (*models.APIKey, error) {
	// Ensure user exists
	var user models.User
	if err := ac.BC.DB.First(&user, "username = ?", username).Error; err != nil {
		return nil, err
	}

	apiKey := models.APIKey{
		Token:       token,
		Description: description,
		Expiracy:    expiracy,
		Username:    username,
		Enabled:     true,
		CreatedBy:   username,
		EditedBy:    username,
	}
	if err := ac.BC.DB.Create(&apiKey).Error; err != nil {
		return nil, err
	}
	apiKey.Token = ""
	return &apiKey, nil
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
		logAudit(&Controller{BC: ac.BC}, r, username, "revoke_api_key", "api_keys", "", http.StatusUnauthorized, nil)
		return
	}

	apiKey := mux.Vars(r)["apiKey"]
	if apiKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing apiKey in path"})
		logAudit(&Controller{BC: ac.BC}, r, username, "revoke_api_key", "api_keys", "", http.StatusBadRequest, nil)
		return
	}

	err := ac.disableAPIKey(username, apiKey)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": true})
		masked := ""
		if len(apiKey) > 8 {
			masked = apiKey[:4] + "...(masked)"
		}
		logAudit(&Controller{BC: ac.BC}, r, username, "revoke_api_key", "api_keys", masked, http.StatusOK, nil)
	case errors.Is(err, gorm.ErrRecordNotFound):
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "API key not found"})
		logAudit(&Controller{BC: ac.BC}, r, username, "revoke_api_key", "api_keys", "", http.StatusNotFound, nil)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(&Controller{BC: ac.BC}, r, username, "revoke_api_key", "api_keys", "", http.StatusInternalServerError, nil)
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

func sanitizeAPIKeyForResponse(key *models.APIKey) models.APIKeyResponse {
	if key == nil {
		return models.APIKeyResponse{}
	}
	return models.APIKeyResponse{
		Description: key.Description,
		Expiracy:    key.Expiracy,
		Enabled:     key.Enabled,
		CreatedAt:   key.CreatedAt,
		LastUsed:    key.LastUsed,
	}
}
