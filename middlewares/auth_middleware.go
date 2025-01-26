package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/r4ulcl/api_template/models"
	"github.com/r4ulcl/api_template/utils"
)

// ContextKey defines a type for context keys to avoid collisions.
type ContextKey string

const (
	// ContextUserID is the key used to store the user ID in the request context.
	ContextUserID ContextKey = "user_id"

	// ContextRole is the key used to store the user's role in the request context.
	ContextRole ContextKey = "role"
)

// AuthMiddleware is a middleware that validates JWT authentication.
//
// It extracts the JWT token from the Authorization header, verifies it,
// and attaches the user ID and role to the request context.
//
// Parameters:
// - secret: The secret key used for JWT signing.
//
// Returns:
// - A middleware function that processes HTTP requests.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Authorization header missing"})
				return
			}

			// Extract token from "Bearer " prefix
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := utils.ParseJWT(tokenString, secret)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid token"})
				return
			}

			// Attach user ID and role to the request context
			ctx := context.WithValue(r.Context(), ContextUserID, claims["user_id"])
			ctx = context.WithValue(ctx, ContextRole, claims["role"])

			// Forward request with modified context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminOnly is a middleware that restricts access to admin users.
//
// It checks the user's role from the request context and denies access
// if the user is not an admin.
//
// Parameters:
// - next: The next HTTP handler to call if access is granted.
//
// Returns:
// - A middleware function that processes HTTP requests.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retrieve user role from context
		role := r.Context().Value(ContextRole)
		if role != "admin" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Forbidden: Admins only"})
			return
		}

		// Forward request if user is an admin
		next.ServeHTTP(w, r)
	})
}
