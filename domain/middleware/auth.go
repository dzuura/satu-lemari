package middleware

import (
	"context"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/dzuura/satu-lemari/domain/common"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// AuthMiddleware handles JWT token validation
type AuthMiddleware struct {
	client *auth.Client
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(client *auth.Client) *AuthMiddleware {
	return &AuthMiddleware{
		client: client,
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
		
		// Verify token with Firebase
		idToken, err := a.client.VerifyIDToken(context.Background(), token)
		if err != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrTokenInvalid, "Invalid or expired token"), common.GenerateTraceID())
			return
		}
		
		// Get user info from Firebase
		user, err := a.client.GetUser(context.Background(), idToken.UID)
		if err != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "User not found"), common.GenerateTraceID())
			return
		}
		
		// TODO: Get user role from database
		// For now, we'll extract it from custom claims or default based on email domain
		role := "user" // default role
		if claims, ok := idToken.Claims["role"]; ok {
			if r, ok := claims.(string); ok {
				role = r
			}
		}
		
		// Set user context
		ctx := context.WithValue(r.Context(), "user_id", idToken.UID)
		ctx = context.WithValue(ctx, "user_email", user.Email)
		ctx = context.WithValue(ctx, "user_role", role)
		ctx = context.WithValue(ctx, "token_claims", idToken.Claims)
		
		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole validates user role for access control
func (a *AuthMiddleware) RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
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
		
		// Verify token with Firebase
		idToken, err := a.client.VerifyIDToken(context.Background(), token)
		if err != nil {
			// Invalid token, continue without authentication
			next.ServeHTTP(w, r)
			return
		}
		
		// Get user info from Firebase
		user, err := a.client.GetUser(context.Background(), idToken.UID)
		if err != nil {
			// User not found, continue without authentication
			next.ServeHTTP(w, r)
			return
		}
		
		// Get user role
		role := "user" // default role
		if claims, ok := idToken.Claims["role"]; ok {
			if r, ok := claims.(string); ok {
				role = r
			}
		}
		
		// Set user context
		ctx := context.WithValue(r.Context(), "user_id", idToken.UID)
		ctx = context.WithValue(ctx, "user_email", user.Email)
		ctx = context.WithValue(ctx, "user_role", role)
		ctx = context.WithValue(ctx, "token_claims", idToken.Claims)
		ctx = context.WithValue(ctx, "authenticated", true)
		
		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts user info from request context
func GetUserFromContext(r *http.Request) (userID, email, role string, authenticated bool) {
	userID, _ = r.Context().Value("user_id").(string)
	email, _ = r.Context().Value("user_email").(string)
	role, _ = r.Context().Value("user_role").(string)
	authenticated, _ = r.Context().Value("authenticated").(bool)
	return
}

// IsAuthenticated checks if request is authenticated
func IsAuthenticated(r *http.Request) bool {
	_, exists := r.Context().Value("user_id").(string)
	return exists
}

// IsPartner checks if authenticated user is a partner
func IsPartner(r *http.Request) bool {
	role, _ := r.Context().Value("user_role").(string)
	return role == "partner" || role == "admin"
}

// IsAdmin checks if authenticated user is an admin
func IsAdmin(r *http.Request) bool {
	role, _ := r.Context().Value("user_role").(string)
	return role == "admin"
}

// IsUser checks if authenticated user is a regular user
func IsUser(r *http.Request) bool {
	role, _ := r.Context().Value("user_role").(string)
	return role == "user"
}