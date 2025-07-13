package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type AuthService struct {
	config *config.Config
	client *auth.Client
	jwt    *JWTService
}

type AuthRequest struct {
	Token    string `json:"token" validate:"required"`
	Type     string `json:"type" validate:"required,oneof=firebase google"`
	Platform string `json:"platform" validate:"required,oneof=web mobile"`
	Username string `json:"username,omitempty"`
}

type AuthResponse struct {
	User        *models.UserProfile `json:"user"`
	AccessToken string              `json:"access_token"`
	TokenType   string              `json:"token_type"`
	ExpiresIn   int64               `json:"expires_in"`
}

func NewAuthService(cfg *config.Config) *AuthService {
	client := initializeFirebaseClient(cfg)
	jwtService := NewJWTService(cfg.JWTSecret, cfg.JWTExpiry)
	return &AuthService{
		config: cfg,
		client: client,
		jwt:    jwtService,
	}
}

func initializeFirebaseClient(cfg *config.Config) *auth.Client {
	// Process private key - remove quotes and handle newlines properly
	privateKey := cfg.FirebasePrivateKey
	if strings.HasPrefix(privateKey, `"`) && strings.HasSuffix(privateKey, `"`) {
		privateKey = strings.Trim(privateKey, `"`)
	}
	privateKey = strings.ReplaceAll(privateKey, "\\n", "\n")

	credMap := map[string]string{
		"type":                        "service_account",
		"project_id":                  cfg.FirebaseProjectID,
		"private_key":                 privateKey,
		"client_email":                cfg.FirebaseClientEmail,
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "",
	}

	credJSON, err := json.Marshal(credMap)
	if err != nil {
		log.Fatalf("Error marshaling Firebase credentials: %v", err)
	}

	app, err := firebase.NewApp(context.Background(), &firebase.Config{
		ProjectID: cfg.FirebaseProjectID,
	}, option.WithCredentialsJSON(credJSON))
	if err != nil {
		log.Fatalf("Error initializing Firebase app: %v", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("Error getting Firebase Auth client: %v", err)
	}

	return client
}

// GetClient exposes Firebase client for middleware
func (s *AuthService) GetClient() *auth.Client {
	return s.client
}

// GetJWTService exposes JWT service for middleware
func (s *AuthService) GetJWTService() *JWTService {
	return s.jwt
}

func (s *AuthService) VerifyAuth(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate request
	if err := s.validateAuthRequest(&req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	ctx := context.Background()
	var user *auth.UserRecord
	var uid string

	// Verify token based on type
	switch req.Type {
	case "firebase":
		var appErr *appError.AppError
		user, uid, appErr = s.verifyFirebaseToken(ctx, req.Token)
		if appErr != nil {
			appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
			return
		}
	case "google":
		var appErr *appError.AppError
		user, uid, appErr = s.verifyGoogleToken(ctx, req.Token)
		if appErr != nil {
			appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
			return
		}
	default:
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Unsupported auth type"),
			common.GenerateTraceID())
		return
	}

	// Determine role based on platform
	role := s.determineRole(req.Platform)

	// Generate username logic:
	// 1. If username is provided in request, use it (manual registration)
	// 2. If no username provided but it's Google auth, generate from Google data
	// 3. If no username provided and it's Firebase auth, leave empty (will be retrieved from DB)
	var username string
	if req.Username != "" {
		// Manual registration flow - use provided username
		username = s.generateUsername(req.Username, user.DisplayName, uid)
	} else if req.Type == "google" {
		// Google auth flow - automatically generate username from Google data
		username = s.generateUsername("", user.DisplayName, uid)
	} else {
		// Firebase auth flow - username will be retrieved from database
		username = "" // Will be set from database
	}

	// Sync user with database
	userProfile, err := s.syncUserWithDatabase(uid, user.Email, username, role)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Generate JWT token with role from database, not platform-based role
	jwtToken, jwtErr := s.jwt.GenerateToken(uid, user.Email, userProfile.Role, userProfile.Username)
	if jwtErr != nil {
		log.Printf("Error generating JWT token: %v", jwtErr)
		appError.WriteErrorResponse(w,
			appError.ErrInternalServer,
			common.GenerateTraceID())
		return
	}

	// Create response
	response := AuthResponse{
		User:        userProfile,
		AccessToken: jwtToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.jwt.GetTokenExpiry().Seconds()),
	}

	common.WriteSuccessResponse(w, response, "Authentication successful")
}

func (s *AuthService) RefreshToken(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from context (set by auth middleware)
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Get user info from context
	email, _ := common.GetUserEmailFromContext(r)
	role, _ := common.GetUserRoleFromContext(r)
	username, _ := common.GetUsernameFromContext(r)

	// Generate new JWT token
	jwtToken, err := s.jwt.GenerateToken(userID, email, role, username)
	if err != nil {
		log.Printf("Error refreshing JWT token: %v", err)
		appError.WriteErrorResponse(w,
			appError.ErrInternalServer,
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"access_token": jwtToken,
		"token_type":   "Bearer",
		"expires_in":   int64(s.jwt.GetTokenExpiry().Seconds()),
	}

	common.WriteSuccessResponse(w, response, "Token refreshed successfully")
}

func (s *AuthService) Logout(w http.ResponseWriter, r *http.Request) {
	// For Firebase, logout is handled client-side
	// This endpoint is for logging purposes and cleanup if needed

	userID, _ := common.GetUserIDFromContext(r)
	log.Printf("User %s logged out", userID)

	common.WriteSuccessResponse(w, nil, "Logout successful")
}

func (s *AuthService) validateAuthRequest(req *AuthRequest) *appError.AppError {
	if req.Token == "" {
		return appError.New(appError.ErrMissingField, "Token is required")
	}
	if req.Type == "" {
		return appError.New(appError.ErrMissingField, "Type is required")
	}
	if req.Platform == "" {
		return appError.New(appError.ErrMissingField, "Platform is required")
	}
	if !common.Contains([]string{"firebase", "google"}, req.Type) {
		return appError.New(appError.ErrInvalidInput, "Invalid auth type")
	}
	if !common.Contains([]string{"web", "mobile"}, req.Platform) {
		return appError.New(appError.ErrInvalidInput, "Invalid platform")
	}
	return nil
}

func (s *AuthService) verifyFirebaseToken(ctx context.Context, token string) (*auth.UserRecord, string, *appError.AppError) {
	idToken, err := s.client.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, "", appError.New(appError.ErrTokenInvalid, "Invalid Firebase token")
	}

	user, err := s.client.GetUser(ctx, idToken.UID)
	if err != nil {
		return nil, "", appError.New(appError.ErrUnauthorized, "User not found")
	}

	return user, idToken.UID, nil
}

func (s *AuthService) verifyGoogleToken(ctx context.Context, token string) (*auth.UserRecord, string, *appError.AppError) {
	// Verify the Google ID token
	idToken, err := s.client.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, "", appError.New(appError.ErrTokenInvalid, "Invalid Google token")
	}

	// Get the user from Firebase
	// The user should already exist since the token was generated by client-side OAuth
	user, err := s.client.GetUser(ctx, idToken.UID)
	if err != nil {
		if auth.IsUserNotFound(err) {
			// This shouldn't happen with proper Google OAuth flow
			// The user should have been created during the OAuth process
			return nil, "", appError.New(appError.ErrUnauthorized, "User not found. Please complete Google Sign-In through the client application.")
		}
		return nil, "", appError.New(appError.ErrUnauthorized, "User verification failed")
	}

	// Check if user has Google as a provider
	hasGoogleProvider := false
	for _, provider := range user.ProviderUserInfo {
		if provider.ProviderID == "google.com" {
			hasGoogleProvider = true
			break
		}
	}

	if !hasGoogleProvider {
		log.Printf("Warning: User %s does not have Google provider", user.UID)
		// This might happen if user was created manually or through other means
		// We'll still allow the authentication but log a warning
	}

	return user, idToken.UID, nil
}

func (s *AuthService) determineRole(platform string) string {
	switch strings.ToLower(platform) {
	case "web":
		return "partner"
	case "mobile":
		return "user"
	default:
		return "user"
	}
}

func (s *AuthService) generateUsername(provided, displayName, uid string) string {
	// If username is provided, clean it to make it valid
	if provided != "" {
		// Clean the provided username: biarkan spasi, hapus karakter non-alfanumerik kecuali spasi
		cleaned := strings.ToLower(provided)
		var result strings.Builder
		for _, char := range cleaned {
			if unicode.IsLetter(char) || unicode.IsDigit(char) || char == ' ' {
				result.WriteRune(char)
			}
		}
		cleaned = strings.TrimSpace(result.String())

		// Ensure minimum length
		if len(cleaned) >= 3 {
			// Truncate if too long
			if len(cleaned) > 30 {
				cleaned = cleaned[:30]
			}
			return cleaned
		}
	}

	// Fallback to display name
	if displayName != "" {
		cleaned := strings.ToLower(displayName)
		var result strings.Builder
		for _, char := range cleaned {
			if unicode.IsLetter(char) || unicode.IsDigit(char) || char == ' ' {
				result.WriteRune(char)
			}
		}
		cleaned = strings.TrimSpace(result.String())

		if len(cleaned) >= 3 {
			if len(cleaned) > 30 {
				cleaned = cleaned[:30]
			}
			return cleaned
		}
	}

	// Final fallback to UID-based username
	return fmt.Sprintf("user_%s", uid[:8])
}

func (s *AuthService) syncUserWithDatabase(uid, email, username, role string) (*models.UserProfile, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Check if user exists by Firebase UID first
	checkURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=*", s.config.SupabaseURL, uid)
	log.Printf("Checking user by UID: %s", uid)
	log.Printf("Check URL: %s", checkURL)

	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	// Use service role key for all database operations to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("UID check failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Database query failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	log.Printf("UID check response: %s", string(body))

	var existingUsers []models.User
	if err := json.Unmarshal(body, &existingUsers); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	log.Printf("Found %d users by UID", len(existingUsers))

	var userProfile *models.UserProfile

	if len(existingUsers) == 0 {
		// User doesn't exist by UID, check if email already exists
		emailCheckURL := fmt.Sprintf("%s/rest/v1/users?email=eq.%s&select=*", s.config.SupabaseURL, email)
		emailReq, err := http.NewRequest("GET", emailCheckURL, nil)
		if err != nil {
			return nil, appError.New(appError.ErrInternal, "Failed to create email check request")
		}

		emailReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
		emailReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
		emailReq.Header.Set("Content-Type", "application/json")

		emailResp, err := client.Do(emailReq)
		if err != nil {
			return nil, appError.New(appError.ErrDatabase, "Database connection failed")
		}
		defer emailResp.Body.Close()

		if emailResp.StatusCode == http.StatusOK {
			emailBody, err := io.ReadAll(emailResp.Body)
			if err != nil {
				return nil, appError.New(appError.ErrInternal, "Failed to read email check response")
			}

			var emailUsers []models.User
			if err := json.Unmarshal(emailBody, &emailUsers); err != nil {
				return nil, appError.New(appError.ErrInternal, "Failed to parse email check response")
			}

			if len(emailUsers) > 0 {
				// Email exists but UID is different - this shouldn't happen with Firebase
				// Return the existing user profile
				user := emailUsers[0]
				userProfile = &models.UserProfile{
					ID:          user.ID,
					Username:    user.Username,
					FullName:    user.FullName,
					Photo:       user.Photo,
					City:        user.City,
					Description: user.Description,
					Role:        user.Role,
					CreatedAt:   user.CreatedAt,
				}
				return userProfile, nil
			}
		}

		// If this is a login flow (no username provided) and user doesn't exist, return error
		if username == "" {
			return nil, appError.New(appError.ErrUnauthorized, "User not found. Please register first.")
		}

		// Create new user (register flow or Google auth with auto-generated username)
		var appErr *appError.AppError
		userProfile, appErr = s.createUser(uid, email, username, role)
		if appErr != nil {
			return nil, appErr
		}
	} else {
		// Return existing user profile
		user := existingUsers[0]
		userProfile = &models.UserProfile{
			ID:          user.ID,
			Username:    user.Username,
			FullName:    user.FullName,
			Photo:       user.Photo,
			City:        user.City,
			Description: user.Description,
			Role:        user.Role,
			CreatedAt:   user.CreatedAt,
		}
	}

	return userProfile, nil
}

func (s *AuthService) createUser(uid, email, username, role string) (*models.UserProfile, *appError.AppError) {
	now := time.Now()
	newUser := map[string]interface{}{
		"id":                    uid,
		"email":                 email,
		"username":              username,
		"role":                  role,
		"is_active":             true,
		"weekly_donation_quota": 3,
		"weekly_donation_used":  0,
		"quota_reset_date":      now.Format("2006-01-02"),
		"created_at":            now.Format(time.RFC3339),
		"updated_at":            now.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(newUser)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal user data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	insertURL := fmt.Sprintf("%s/rest/v1/users", s.config.SupabaseURL)
	req, err := http.NewRequest("POST", insertURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	// Use service role key for INSERT operations to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := client.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to create user")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to create user: %s", string(bodyBytes))

		// Check if it's a duplicate key error
		if strings.Contains(string(bodyBytes), "duplicate key value violates unique constraint") {
			// Try to get the existing user by email
			client := &http.Client{Timeout: 10 * time.Second}
			emailCheckURL := fmt.Sprintf("%s/rest/v1/users?email=eq.%s&select=*", s.config.SupabaseURL, email)
			emailReq, err := http.NewRequest("GET", emailCheckURL, nil)
			if err != nil {
				return nil, appError.New(appError.ErrInternal, "Failed to create email check request")
			}

			emailReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
			emailReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
			emailReq.Header.Set("Content-Type", "application/json")

			emailResp, err := client.Do(emailReq)
			if err != nil {
				return nil, appError.New(appError.ErrDatabase, "Database connection failed")
			}
			defer emailResp.Body.Close()

			if emailResp.StatusCode == http.StatusOK {
				emailBody, err := io.ReadAll(emailResp.Body)
				if err != nil {
					return nil, appError.New(appError.ErrInternal, "Failed to read email check response")
				}

				var emailUsers []models.User
				if err := json.Unmarshal(emailBody, &emailUsers); err != nil {
					return nil, appError.New(appError.ErrInternal, "Failed to parse email check response")
				}

				if len(emailUsers) > 0 {
					// Return the existing user profile
					user := emailUsers[0]
					return &models.UserProfile{
						ID:          user.ID,
						Username:    user.Username,
						FullName:    user.FullName,
						Photo:       user.Photo,
						City:        user.City,
						Description: user.Description,
						Role:        user.Role,
						CreatedAt:   user.CreatedAt,
					}, nil
				}
			}
		}

		return nil, appError.New(appError.ErrDatabase, "Failed to create user in database")
	}

	// Parse created user
	var createdUsers []models.User
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	// Debug: log the response
	log.Printf("Supabase response: %s", string(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &createdUsers); err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		log.Printf("Response body: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrInternal, "Failed to parse created user")
	}

	if len(createdUsers) == 0 {
		log.Printf("No users returned from creation, response: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrInternal, "No user returned from creation")
	}

	user := createdUsers[0]
	return &models.UserProfile{
		ID:          user.ID,
		Username:    user.Username,
		FullName:    user.FullName,
		Photo:       user.Photo,
		City:        user.City,
		Description: user.Description,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}, nil
}
