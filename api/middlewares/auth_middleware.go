package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// ContextKey defines a type for context keys to avoid collisions.
type ContextKey string

const (
	// ContextUserID is the key used to store the username in the request context.
	ContextUserID ContextKey = "user_id"

	// ContextRole is the key used to store the user's role in the request context.
	ContextRole ContextKey = "role"
)

func AuthMiddleware(secret string, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			apiKeyHeader := strings.TrimSpace(r.Header.Get("X-API-Key"))

			// Pick a candidate token from either header
			var tokenStr string
			switch {
			case strings.HasPrefix(authHeader, "Bearer "):
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			case strings.HasPrefix(authHeader, "ApiKey "):
				tokenStr = strings.TrimPrefix(authHeader, "ApiKey ")
			case authHeader != "":
				tokenStr = authHeader // bare token accepted
			case apiKeyHeader != "":
				tokenStr = apiKeyHeader
			default:
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Missing authentication credentials"})
				return
			}

			// Parse once without exp validation to read claims safely
			claims, err := utils.ParseJWTNoExpiry(tokenStr, secret)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid token"})
				return
			}

			username, role, ok := extractUserAndRole(claims)
			if !ok || username == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token missing required claims"})
				return
			}

			// If token has exp, enforce expiry and treat as Bearer
			if exp, ok := getExpUnix(claims); ok {
				now := time.Now().Unix()
				if now >= exp {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Token expired"})
					return
				}
				// Valid exp-based JWT accepted as Bearer
				ctx := context.WithValue(r.Context(), ContextUserID, username)
				ctx = context.WithValue(ctx, ContextRole, role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// No exp present, treat as API key and verify in DB
			var rec models.APIKey
			if err := db.First(&rec, "token = ? AND username = ? AND enabled = ?", tokenStr, username, true).Error; err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "API key not recognized or disabled"})
				return
			}
			_ = db.Model(&rec).Update("last_used", time.Now()).Error

			ctx := context.WithValue(r.Context(), ContextUserID, username)
			ctx = context.WithValue(ctx, ContextRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getExpUnix extracts the exp claim as a Unix timestamp if present.
func getExpUnix(claims map[string]interface{}) (int64, bool) {
	v, ok := claims["exp"]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, true
		}
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
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
