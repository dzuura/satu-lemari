package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/common"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// AuthMiddleware handles JWT token validation
type AuthMiddleware struct {
	jwtService *auth.JWTService
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(jwtService *auth.JWTService) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService: jwtService,
	}
}

// RequireAuth validates JWT token and sets user context
func (a *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			appError.WriteErrorResponse(w, appError.ErrInvalidCredentials, common.GenerateTraceID())
			return
		}

		// Check Bearer token format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			appError.WriteErrorResponse(w, appError.New(appError.ErrTokenInvalid, "Invalid token format"), common.GenerateTraceID())
			return
		}

		token := parts[1]

		// Validate JWT token
		claims, err := a.jwtService.ValidateToken(token)
		if err != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrTokenInvalid, "Invalid or expired token"), common.GenerateTraceID())
			return
		}

		// Log token claims for debugging
		log.Printf("JWT Token validated - UserID: %s, Email: %s, Role: %s, Username: %s",
			claims.UserID, claims.Email, claims.Role, claims.Username)

		// Set user context using proper context keys
		ctx := context.WithValue(r.Context(), common.UserIDKey(), claims.UserID)
		ctx = context.WithValue(ctx, common.UserEmailKey(), claims.Email)
		ctx = context.WithValue(ctx, common.UserRoleKey(), claims.Role)
		ctx = context.WithValue(ctx, common.UsernameKey(), claims.Username)
		ctx = context.WithValue(ctx, common.AuthenticatedKey(), true)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole validates user role for access control
func (a *AuthMiddleware) RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user role from context (should be set by RequireAuth middleware)
			role, ok := r.Context().Value(common.UserRoleKey()).(string)
			if !ok {
				log.Printf("Role not found in context")
				appError.WriteErrorResponse(w, appError.ErrAccessDenied, common.GenerateTraceID())
				return
			}

			// Log role check for debugging
			log.Printf("Role check - User role: %s, Allowed roles: %v", role, allowedRoles)

			// Check if user role is allowed
			roleAllowed := false
			for _, allowedRole := range allowedRoles {
				if role == allowedRole {
					roleAllowed = true
					break
				}
			}

			if !roleAllowed {
				log.Printf("Access denied - User role: %s not in allowed roles: %v", role, allowedRoles)
				appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Insufficient permissions"), common.GenerateTraceID())
				return
			}

			log.Printf("Access granted - User role: %s is allowed", role)
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePartner middleware for partner-only endpoints
func (a *AuthMiddleware) RequirePartner(next http.Handler) http.Handler {
	return a.RequireRole("partner", "admin")(next)
}

// RequireAdmin middleware for admin-only endpoints
func (a *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return a.RequireRole("admin")(next)
}

// RequireUser middleware for user-only endpoints
func (a *AuthMiddleware) RequireUser(next http.Handler) http.Handler {
	return a.RequireRole("user", "admin")(next)
}

// OptionalAuth validates token if present but doesn't require it
func (a *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// No token provided, continue without authentication
			next.ServeHTTP(w, r)
			return
		}

		// Check Bearer token format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			// Invalid format, continue without authentication
			next.ServeHTTP(w, r)
			return
		}

		token := parts[1]

		// Validate JWT token
		claims, err := a.jwtService.ValidateToken(token)
		if err != nil {
			// Invalid token, continue without authentication
			next.ServeHTTP(w, r)
			return
		}

		// Set user context using proper context keys
		ctx := context.WithValue(r.Context(), common.UserIDKey(), claims.UserID)
		ctx = context.WithValue(ctx, common.UserEmailKey(), claims.Email)
		ctx = context.WithValue(ctx, common.UserRoleKey(), claims.Role)
		ctx = context.WithValue(ctx, common.UsernameKey(), claims.Username)
		ctx = context.WithValue(ctx, common.AuthenticatedKey(), true)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts user info from request context
func GetUserFromContext(r *http.Request) (userID, email, role string, authenticated bool) {
	userID, _ = r.Context().Value(common.UserIDKey()).(string)
	email, _ = r.Context().Value(common.UserEmailKey()).(string)
	role, _ = r.Context().Value(common.UserRoleKey()).(string)
	authenticated, _ = r.Context().Value(common.AuthenticatedKey()).(bool)
	return
}

// IsAuthenticated checks if request is authenticated
func IsAuthenticated(r *http.Request) bool {
	_, exists := r.Context().Value(common.UserIDKey()).(string)
	return exists
}

// IsPartner checks if authenticated user is a partner
func IsPartner(r *http.Request) bool {
	role, _ := r.Context().Value(common.UserRoleKey()).(string)
	return role == "partner" || role == "admin"
}

// IsAdmin checks if authenticated user is an admin
func IsAdmin(r *http.Request) bool {
	role, _ := r.Context().Value(common.UserRoleKey()).(string)
	return role == "admin"
}

// IsUser checks if authenticated user is a regular user
func IsUser(r *http.Request) bool {
	role, _ := r.Context().Value(common.UserRoleKey()).(string)
	return role == "user"
}
