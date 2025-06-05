package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
)

// ContextKey defines a type for context keys to avoid collisions.
type ContextKey string

const (
	// ContextUserID is the key used to store the username in the request context.
	ContextUserID ContextKey = "user_id"

	// ContextRole is the key used to store the user's role in the request context.
	ContextRole ContextKey = "role"
)

// AuthMiddleware is a middleware that validates JWT authentication.
// It extracts the JWT token from the Authorization header, verifies it,
// and attaches the username and role to the request context.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Authorization header missing"})
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := utils.ParseJWT(tokenString, secret)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid token"})
				return
			}

			// Extract "username" and "role" from claims
			usernameVal, userOK := claims["username"]
			roleVal, roleOK := claims["role"]
			if !userOK || !roleOK {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token missing required claims"})
				return
			}

			username, ok1 := usernameVal.(string)
			role, ok2 := roleVal.(string)
			if !ok1 || !ok2 || username == "" || role == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid claims in token"})
				return
			}

			// Attach username and role to the request context
			ctx := context.WithValue(r.Context(), ContextUserID, username)
			ctx = context.WithValue(ctx, ContextRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RoleMiddleware restricts access to users whose role is one of the allowedRoles.
// It reads the role from the request context (ContextRole) and returns 403 if no match.
func RoleMiddleware(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the role from context
			roleVal := r.Context().Value(ContextRole)
			role, _ := roleVal.(string)

			// Check if the user’s role is in the allowed list
			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			// If no match, forbid
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: "Forbidden: insufficient permissions",
			})
		})
	}
}
