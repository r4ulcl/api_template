package models

import "time"

// LoginRequest represents the request payload for user authentication.
//
// It contains the username and password fields, both of which are required.
type LoginRequest struct {
	// Username is the unique identifier for the user attempting to log in.
	Username string `binding:"required" json:"username"`

	// Password is the user's password used for authentication.
	Password string `binding:"required" json:"password"`

	// TotpCode is required when Time-based OTP is enabled for the account.
	TotpCode string `json:"totp_code,omitempty"`
}

// JWTResponse represents the response containing a JWT token.
//
// This is returned to the client upon successful authentication.
type JWTResponse struct {
	// Token is the JWT token assigned to the authenticated user.
	Token string `json:"token"`
}

// RegisterRequest represents the request payload for user registration.
//
// It contains the username, password, and role of the new user.
type RegisterRequest struct {
	// Username is the unique identifier for the new user.
	Username string `binding:"required" json:"username"`

	// Password is the new user's password, which will be hashed before storage.
	Password string `binding:"required" json:"password"`

	// Role specifies whether the user is an "admin" or "user".
	Role Role `json:"role"`
}

// DefaultRequest represents a minimal request structure.
//
// It contains a single field, which can be used for generic request handling.
type DefaultRequest struct {
	// Field is a placeholder for data that might be required in some requests.
	Field string `json:"field"`
}

// ErrorResponse represents an error message response.
//
// It is used to return structured error messages to the client.
type ErrorResponse struct {
	// Error contains a descriptive error message.
	Error string `json:"error"`
}

// CreateAPIKeyRequest represents the payload required to create a new API key.
type CreateAPIKeyRequest struct {
	// Description is a human readable label to identify the key's intent.
	Description string `json:"description" binding:"required"`
	// Expiracy is an optional UTC timestamp when the key becomes invalid.
	Expiracy *time.Time `json:"expiracy,omitempty"`
}

// APIKeyResponse exposes non-sensitive fields associated with an API key.
type APIKeyResponse struct {
	APIKey      string     `json:"api_key,omitempty"`
	Description string     `json:"description"`
	Expiracy    *time.Time `json:"expiracy,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
}
