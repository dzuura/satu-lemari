package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/common"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// JWTAuthMiddleware handles JWT token validation
type JWTAuthMiddleware struct {
	jwtService *auth.JWTService
}

// NewJWTAuthMiddleware creates a new JWT auth middleware
func NewJWTAuthMiddleware(jwtService *auth.JWTService) *JWTAuthMiddleware {
	return &JWTAuthMiddleware{
		jwtService: jwtService,
	}
}

// RequireAuth validates JWT token and sets user context
func (j *JWTAuthMiddleware) RequireAuth(next http.Handler) http.Handler {
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
		claims, err := j.jwtService.ValidateToken(token)
		if err != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrTokenInvalid, "Invalid or expired token"), common.GenerateTraceID())
			return
		}

		// Set user context
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "user_email", claims.Email)
		ctx = context.WithValue(ctx, "user_role", claims.Role)
		ctx = context.WithValue(ctx, "username", claims.Username)
		ctx = context.WithValue(ctx, "authenticated", true)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole validates user role for access control
func (j *JWTAuthMiddleware) RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user role from context (should be set by RequireAuth middleware)
			role, ok := r.Context().Value("user_role").(string)
			if !ok {
				appError.WriteErrorResponse(w, appError.ErrAccessDenied, common.GenerateTraceID())
				return
			}

			// Check if user role is allowed
			roleAllowed := false
			for _, allowedRole := range allowedRoles {
				if role == allowedRole {
					roleAllowed = true
					break
				}
			}

			if !roleAllowed {
				appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Insufficient permissions"), common.GenerateTraceID())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePartner middleware for partner-only endpoints
func (j *JWTAuthMiddleware) RequirePartner(next http.Handler) http.Handler {
	return j.RequireRole("partner", "admin")(next)
}

// RequireAdmin middleware for admin-only endpoints
func (j *JWTAuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return j.RequireRole("admin")(next)
}

// RequireUser middleware for user-only endpoints
func (j *JWTAuthMiddleware) RequireUser(next http.Handler) http.Handler {
	return j.RequireRole("user", "admin")(next)
}

// OptionalAuth validates token if present but doesn't require it
func (j *JWTAuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
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
		claims, err := j.jwtService.ValidateToken(token)
		if err != nil {
			// Invalid token, continue without authentication
			next.ServeHTTP(w, r)
			return
		}

		// Set user context
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "user_email", claims.Email)
		ctx = context.WithValue(ctx, "user_role", claims.Role)
		ctx = context.WithValue(ctx, "username", claims.Username)
		ctx = context.WithValue(ctx, "authenticated", true)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
