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

type meUpdateError struct {
	status  int
	message string
	change  *auditChange
}

func newMeUpdateError(status int, message string, change *auditChange) *meUpdateError {
	return &meUpdateError{status: status, message: message, change: change}
}

type meUpdateContext struct {
	controller    *AuthController
	w             http.ResponseWriter
	r             *http.Request
	username      string
	role          models.Role
	user          models.User
	payload       map[string]interface{}
	request       models.UpdateUser
	before        models.User
	secretChanged bool
	newTotpSecret string
}

func (m *meUpdateContext) audit(status int, change *auditChange) {
	logAudit(&Controller{BC: m.controller.BC}, m.r, m.username, "me_update", "users", m.username, status, change)
}

func (m *meUpdateContext) writeError(err *meUpdateError) {
	m.w.WriteHeader(err.status)
	_ = json.NewEncoder(m.w).Encode(models.ErrorResponse{Error: err.message})
	m.audit(err.status, err.change)
}

func (m *meUpdateContext) populateAuthContext() *meUpdateError {
	uidVal := m.r.Context().Value(middlewares.ContextUserID)
	rlVal := m.r.Context().Value(middlewares.ContextRole)
	username, _ := uidVal.(string)
	roleStr, _ := rlVal.(string)
	m.username = strings.TrimSpace(username)
	m.role = models.Role(strings.TrimSpace(roleStr))
	if m.username == "" {
		return newMeUpdateError(http.StatusUnauthorized, "Unauthorized", nil)
	}
	return nil
}

func (m *meUpdateContext) loadCurrentUser() *meUpdateError {
	if err := m.controller.BC.GetRecordsByIDWithSensitive(&m.user, m.username); err != nil {
		return newMeUpdateError(http.StatusNotFound, "User not found", nil)
	}
	m.user.TotpSecret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(m.user.TotpSecret), " ", ""))
	if m.user.TotpSecret == "" {
		m.user.TotpEnabled = false
	}
	m.before = sanitizeUserForAudit(m.user)
	return nil
}

func (m *meUpdateContext) decodeRequestBody() *meUpdateError {
	const maxBody = 1 << 20 // 1 MiB
	m.r.Body = http.MaxBytesReader(m.w, m.r.Body, maxBody)
	m.payload = make(map[string]interface{})
	if err := json.NewDecoder(m.r.Body).Decode(&m.payload); err != nil {
		return newMeUpdateError(http.StatusBadRequest, "Invalid JSON", &auditChange{Before: m.before})
	}
	bodyBytes, _ := json.Marshal(m.payload)
	_ = json.Unmarshal(bodyBytes, &m.request)
	return nil
}

func (m *meUpdateContext) validateFields() *meUpdateError {
	disallowedAlways := map[string]bool{
		"username":    true,
		"created_at":  true,
		"last_update": true,
		"created_by":  true,
		"edited_by":   true,
	}

	allowed, ok := models.UpdateUserWhitelist[m.role]
	if !ok {
		allowed = models.UpdateUserWhitelist[models.DefaultRoleWhitelist]
	}

	for key := range m.payload {
		if disallowedAlways[key] {
			return newMeUpdateError(http.StatusBadRequest, fmt.Sprintf("cannot update field %q via this endpoint", key), &auditChange{Before: m.before})
		}
		if m.role == models.AdminRole {
			continue
		}
		if !allowed[key] {
			return newMeUpdateError(http.StatusBadRequest, fmt.Sprintf("cannot update field %q", key), &auditChange{Before: m.before})
		}
	}
	return nil
}

func (m *meUpdateContext) applyRoleSpecificUpdates() *meUpdateError {
	if m.role == models.AdminRole {
		return m.applyAdminUpdates()
	}
	return m.applyStandardUserUpdates()
}

func (m *meUpdateContext) applyAdminUpdates() *meUpdateError {
	if _, ok := m.payload["email"]; ok {
		m.user.Email = strings.TrimSpace(m.request.Email)
	}
	if _, ok := m.payload["email_verified"]; ok {
		m.user.EmailVerified = m.request.EmailVerified
	}
	if _, ok := m.payload["role"]; ok {
		roleStr := strings.TrimSpace(string(m.request.Role))
		switch models.Role(roleStr) {
		case models.AdminRole, models.UserRole:
			m.user.Role = models.Role(roleStr)
		default:
			return newMeUpdateError(http.StatusBadRequest, "invalid role value", &auditChange{Before: m.before})
		}
	}
	return nil
}

func (m *meUpdateContext) applyStandardUserUpdates() *meUpdateError {
	if _, ok := m.payload["email"]; ok {
		newEmail := strings.TrimSpace(m.request.Email)
		if newEmail == "" {
			return newMeUpdateError(http.StatusBadRequest, "email cannot be empty", &auditChange{Before: m.before})
		}
		if newEmail != m.user.Email {
			m.user.Email = newEmail
			m.user.EmailVerified = false
		}
	}
	return nil
}

func (m *meUpdateContext) updatePassword() *meUpdateError {
	if _, ok := m.payload["new_password"]; !ok || strings.TrimSpace(m.request.NewPassword) == "" {
		return nil
	}

	if m.role == models.AdminRole {
		return m.setPasswordDirectly(m.request.NewPassword)
	}
	return m.changePasswordWithVerification()
}

func (m *meUpdateContext) setPasswordDirectly(newPassword string) *meUpdateError {
	hashed, err := utils.HashPassword(newPassword)
	if err != nil {
		return newMeUpdateError(http.StatusInternalServerError, "failed to hash new password", &auditChange{Before: m.before})
	}
	m.user.Password = hashed
	return nil
}

func (m *meUpdateContext) changePasswordWithVerification() *meUpdateError {
	currRaw, hasCurr := m.payload["password"]
	currPassword, _ := currRaw.(string)
	if !hasCurr || strings.TrimSpace(currPassword) == "" {
		return newMeUpdateError(http.StatusBadRequest, "current password required", &auditChange{Before: m.before})
	}
	if err := utils.CheckPassword(m.user.Password, currPassword); err != nil {
		return newMeUpdateError(http.StatusUnauthorized, "current password incorrect", &auditChange{Before: m.before})
	}
	return m.setPasswordDirectly(m.request.NewPassword)
}

func (m *meUpdateContext) updateTotp() *meUpdateError {
	if err := m.handleTotpSecret(); err != nil {
		return err
	}
	if err := m.handleTotpReset(); err != nil {
		return err
	}
	if err := m.handleTotpToggle(); err != nil {
		return err
	}
	if m.user.TotpEnabled && strings.TrimSpace(m.user.TotpSecret) == "" {
		return newMeUpdateError(http.StatusBadRequest, "TOTP secret must be set before enabling", &auditChange{Before: m.before})
	}
	return nil
}

func (m *meUpdateContext) handleTotpSecret() *meUpdateError {
	if _, ok := m.payload["totp_secret"]; !ok {
		return nil
	}
	currentSecret := m.user.TotpSecret
	sanitized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(m.request.TotpSecret), " ", ""))
	if sanitized != currentSecret {
		m.secretChanged = true
	}
	m.user.TotpSecret = sanitized
	if sanitized == "" || sanitized != currentSecret {
		m.user.TotpEnabled = false
	}
	return nil
}

func (m *meUpdateContext) handleTotpReset() *meUpdateError {
	rawReset, hasReset := m.payload["totp_reset_secret"]
	if !hasReset {
		return nil
	}
	flag, ok := rawReset.(bool)
	if !ok || !flag {
		return nil
	}
	if m.request.TotpEnabled != nil && !*m.request.TotpEnabled {
		return newMeUpdateError(http.StatusBadRequest, "cannot reset TOTP secret while disabling", &auditChange{Before: m.before})
	}
	if m.user.TotpEnabled {
		return newMeUpdateError(http.StatusBadRequest, "disable TOTP before generating a new secret", &auditChange{Before: m.before})
	}
	secret, err := utils.GenerateTOTPSecret()
	if err != nil {
		return newMeUpdateError(http.StatusInternalServerError, "failed to generate TOTP secret", &auditChange{Before: m.before})
	}
	m.user.TotpSecret = secret
	m.user.TotpEnabled = false
	m.secretChanged = true
	m.newTotpSecret = secret
	return nil
}

func (m *meUpdateContext) handleTotpToggle() *meUpdateError {
	if _, ok := m.payload["totp_enabled"]; !ok {
		return nil
	}
	if m.request.TotpEnabled == nil {
		return newMeUpdateError(http.StatusBadRequest, "totp_enabled requires a boolean value", &auditChange{Before: m.before})
	}
	if *m.request.TotpEnabled {
		return m.enableTotp()
	}
	return m.disableTotp()
}

func (m *meUpdateContext) enableTotp() *meUpdateError {
	if strings.TrimSpace(m.user.TotpSecret) == "" {
		return newMeUpdateError(http.StatusBadRequest, "generate a TOTP secret before enabling", &auditChange{Before: m.before})
	}
	code := m.sanitizeTotpCode(m.request.TotpCode)
	if code == "" {
		return newMeUpdateError(http.StatusBadRequest, "totp_code required when enabling", &auditChange{Before: m.before})
	}
	if !utils.ValidateTOTP(m.user.TotpSecret, code) {
		return newMeUpdateError(http.StatusBadRequest, "invalid TOTP code", &auditChange{Before: m.before})
	}
	m.user.TotpEnabled = true
	return nil
}

func (m *meUpdateContext) disableTotp() *meUpdateError {
	if strings.TrimSpace(m.user.TotpSecret) == "" {
		return newMeUpdateError(http.StatusBadRequest, "TOTP is not currently configured", &auditChange{Before: m.before})
	}
	code := m.sanitizeTotpCode(m.request.TotpCode)
	if code == "" {
		return newMeUpdateError(http.StatusBadRequest, "totp_code required when disabling", &auditChange{Before: m.before})
	}
	if !utils.ValidateTOTP(m.user.TotpSecret, code) {
		return newMeUpdateError(http.StatusBadRequest, "invalid TOTP code", &auditChange{Before: m.before})
	}
	m.user.TotpEnabled = false
	return nil
}

func (m *meUpdateContext) sanitizeTotpCode(raw string) string {
	raw = strings.TrimSpace(raw)
	var builder strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (m *meUpdateContext) persistChanges() *meUpdateError {
	m.user.Username = m.username
	m.user.EditedBy = m.username
	m.user.LastUpdate = time.Now()
	updates := map[string]interface{}{
		"edited_by":   m.user.EditedBy,
		"last_update": m.user.LastUpdate,
	}

	if m.secretChanged {
		updates["totp_secret"] = m.user.TotpSecret
		updates["totp_enabled"] = m.user.TotpEnabled
	} else if _, ok := m.payload["totp_enabled"]; ok {
		updates["totp_enabled"] = m.user.TotpEnabled
	}

	if m.role == models.AdminRole {
		if _, ok := m.payload["email"]; ok {
			updates["email"] = m.user.Email
		}
		if _, ok := m.payload["email_verified"]; ok {
			updates["email_verified"] = m.user.EmailVerified
		}
		if _, ok := m.payload["role"]; ok {
			updates["role"] = m.user.Role
		}
	} else {
		if _, ok := m.payload["email"]; ok {
			updates["email"] = m.user.Email
			updates["email_verified"] = m.user.EmailVerified
		}
	}

	if _, ok := m.payload["new_password"]; ok && strings.TrimSpace(m.request.NewPassword) != "" {
		updates["password"] = m.user.Password
	}

	if len(updates) == 2 {
		m.writeSuccess()
		return nil
	}

	if err := m.controller.BC.DB.Model(&models.User{}).
		Where("username = ?", m.user.Username).
		Updates(updates).Error; err != nil {
		return newMeUpdateError(http.StatusInternalServerError, err.Error(), &auditChange{Before: m.before})
	}

	m.writeSuccess()
	return nil
}

func (m *meUpdateContext) writeSuccess() {
	m.user.Password = ""
	m.user.TotpSecret = ""
	resp := struct {
		models.User
		NewTotpSecret string `json:"new_totp_secret,omitempty"`
	}{
		User: m.user,
	}
	if m.newTotpSecret != "" {
		resp.NewTotpSecret = m.newTotpSecret
	}
	m.w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(m.w).Encode(resp)
	m.audit(http.StatusOK, &auditChange{
		Before: m.before,
		After:  sanitizeUserForAudit(m.user),
	})
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
	ctx := &meUpdateContext{controller: ac, w: w, r: r}
	if err := ctx.populateAuthContext(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.loadCurrentUser(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.decodeRequestBody(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.validateFields(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.applyRoleSpecificUpdates(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.updatePassword(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.updateTotp(); err != nil {
		ctx.writeError(err)
		return
	}
	if err := ctx.persistChanges(); err != nil {
		ctx.writeError(err)
	}
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

	// Restrict TOTP secret visibility to admins only
	roleVal := r.Context().Value(middlewares.ContextRole)
	roleStr, _ := roleVal.(string)
	if models.Role(strings.TrimSpace(roleStr)) != models.AdminRole {
		user.TotpSecret = ""
	} else {
		user.TotpSecret = strings.TrimSpace(user.TotpSecret)
	}

	// Clear other sensitive fields
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
