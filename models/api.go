package models

// LoginRequest represents the request payload for user authentication.
//
// It contains the username and password fields, both of which are required.
type LoginRequest struct {
	// Username is the unique identifier for the user attempting to log in.
	Username string `json:"username" binding:"required"`

	// Password is the user's password used for authentication.
	Password string `json:"password" binding:"required"`
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
	Username string `json:"username" binding:"required"`

	// Password is the new user's password, which will be hashed before storage.
	Password string `json:"password" binding:"required"`

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
