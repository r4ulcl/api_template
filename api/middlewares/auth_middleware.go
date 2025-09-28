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

// AuthMiddleware validates either:
// - Bearer JWT with exp validation
// - API key JWT (no expiry) provided as Authorization: ApiKey <token> or X-API-Key: <token>
// On success it sets ContextUserID and ContextRole.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeader := r.Header.Get("Authorization")
			apiKeyHeader := r.Header.Get("X-API-Key")

			// 1) Bearer JWT with standard validation
			if authHeader != "" {
				tokenString := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := utils.ParseJWT(tokenString, secret)
				if err != nil {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid token"})
					return
				}
				username, role, ok := extractUserAndRole(claims)
				if !ok {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token missing required claims"})
					return
				}
				ctx := context.WithValue(r.Context(), ContextUserID, username)
				ctx = context.WithValue(ctx, ContextRole, role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2) API key JWT (no expiry). Accept two header styles.
			apiKey := ""
			if strings.HasPrefix(authHeader, "ApiKey ") {
				apiKey = strings.TrimPrefix(authHeader, "ApiKey ")
			} else if apiKeyHeader != "" {
				apiKey = apiKeyHeader
			}
			if apiKey != "" {
				claims, err := utils.ParseJWTNoExpiry(apiKey, secret) // must skip exp validation
				if err != nil {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid API key"})
					return
				}
				username, role, ok := extractUserAndRole(claims)
				if !ok {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "API key missing required claims"})
					return
				}
				ctx := context.WithValue(r.Context(), ContextUserID, username)
				ctx = context.WithValue(ctx, ContextRole, role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 3) No supported credentials
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing authentication credentials"})
		})
	}
}

// extractUserAndRole pulls username and role out of JWT claims.
func extractUserAndRole(claims map[string]interface{}) (username string, role string, ok bool) {
	u, uok := claims["username"]
	r, rok := claims["role"]
	us, uok2 := u.(string)
	rs, rok2 := r.(string)
	if !uok || !rok || !uok2 || !rok2 || us == "" || rs == "" {
		return "", "", false
	}
	return us, rs, true
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

// OwnScopeMiddleware allows full roles unrestricted access
// and limits "own" roles to their own data by setting a context flag.
func OwnScopeMiddleware(fullRoles []string, ownRoles []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roleVal := r.Context().Value(ContextRole)
			role, _ := roleVal.(string)
			if role == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing role in context"})
				return
			}

			isFull := false
			for _, fr := range fullRoles {
				if role == fr {
					isFull = true
					break
				}
			}

			isOwn := false
			for _, or := range ownRoles {
				if role == or {
					isOwn = true
					break
				}
			}

			// If neither allowed fully nor own, forbid
			if !isFull && !isOwn {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{
					Error: "Forbidden: insufficient permissions",
				})
				return
			}

			// If only own access, mark context accordingly
			ctx := r.Context()
			if isOwn && !isFull {
				ctx = context.WithValue(ctx, ContextOwnOnly, true)
			} else {
				ctx = context.WithValue(ctx, ContextOwnOnly, false)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ContextOwnOnly indicates if the request is restricted to "own" data.
const ContextOwnOnly ContextKey = "own_only"

// IsOwnOnly checks if the current request context is limited to "own" access.
func IsOwnOnly(ctx context.Context) bool {
	val := ctx.Value(ContextOwnOnly)
	ownOnly, _ := val.(bool)
	return ownOnly
}
