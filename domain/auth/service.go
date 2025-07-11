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

	"firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/gorilla/mux"
	"google.golang.org/api/option"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type AuthService struct {
	config *config.Config
	client *auth.Client
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
	return &AuthService{
		config: cfg,
		client: client,
	}
}

func initializeFirebaseClient(cfg *config.Config) *auth.Client {
	credMap := map[string]string{
		"type":                        "service_account",
		"project_id":                  cfg.FirebaseProjectID,
		"private_key":                 strings.ReplaceAll(cfg.FirebasePrivateKey, "\\n", "\n"),
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

func (s *AuthService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/auth/verify", s.VerifyAuth).Methods("POST")
	r.HandleFunc("/auth/refresh", s.RefreshToken).Methods("POST")
	r.HandleFunc("/auth/logout", s.Logout).Methods("POST")
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
		var err error
		user, uid, err = s.verifyFirebaseToken(ctx, req.Token)
		if err != nil {
			appError.WriteErrorResponse(w, err, common.GenerateTraceID())
			return
		}
	case "google":
		var err error
		user, uid, err = s.verifyGoogleToken(ctx, req.Token)
		if err != nil {
			appError.WriteErrorResponse(w, err, common.GenerateTraceID())
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

	// Generate username if not provided
	username := s.generateUsername(req.Username, user.DisplayName, uid)

	// Create custom token for API access
	customToken, err := s.client.CustomToken(ctx, uid)
	if err != nil {
		log.Printf("Error creating custom token: %v", err)
		appError.WriteErrorResponse(w,
			appError.ErrInternalServer,
			common.GenerateTraceID())
		return
	}

	// Sync user with database
	userProfile, err := s.syncUserWithDatabase(uid, user.Email, username, role)
	if err != nil {
		log.Printf("Error syncing user with database: %v", err)
		appError.WriteErrorResponse(w,
			appError.ErrInternalServer,
			common.GenerateTraceID())
		return
	}

	// Create response
	response := AuthResponse{
		User:        userProfile,
		AccessToken: customToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600, // 1 hour
	}

	common.WriteSuccessResponse(w, response, "Authentication successful")
}

func (s *AuthService) RefreshToken(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from context (set by auth middleware)
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	// Generate new custom token
	customToken, err := s.client.CustomToken(context.Background(), userID)
	if err != nil {
		log.Printf("Error refreshing token: %v", err)
		appError.WriteErrorResponse(w,
			appError.ErrInternalServer,
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"access_token": customToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
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
	idToken, err := s.client.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, "", appError.New(appError.ErrTokenInvalid, "Invalid Google token")
	}

	user, err := s.client.GetUser(ctx, idToken.UID)
	if err != nil {
		if auth.IsUserNotFound(err) {
			// Create new user if not found
			user, err = s.client.CreateUser(ctx, (&auth.UserToCreate{}).
				UID(idToken.UID).
				Email(idToken.Claims["email"].(string)).
				DisplayName(idToken.Claims["name"].(string)))
			if err != nil {
				return nil, "", appError.New(appError.ErrInternal, "Failed to create user")
			}
		} else {
			return nil, "", appError.New(appError.ErrUnauthorized, "User verification failed")
		}
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
	if provided != "" && common.IsValidUsername(provided) {
		return provided
	}
	if displayName != "" {
		// Clean display name to make it a valid username
		cleaned := strings.ReplaceAll(strings.ToLower(displayName), " ", "_")
		if common.IsValidUsername(cleaned) {
			return cleaned
		}
	}
	return fmt.Sprintf("user_%s", uid[:8])
}

func (s *AuthService) syncUserWithDatabase(uid, email, username, role string) (*models.UserProfile, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Check if user exists
	checkURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=*", s.config.SupabaseURL, uid)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrDatabase, "Database query failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	var existingUsers []models.User
	if err := json.Unmarshal(body, &existingUsers); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	var userProfile *models.UserProfile

	if len(existingUsers) == 0 {
		// Create new user
		userProfile, err = s.createUser(uid, email, username, role)
		if err != nil {
			return nil, err
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
	newUser := map[string]interface{}{
		"id":         uid,
		"email":      email,
		"username":   username,
		"role":       role,
		"is_active":  true,
		"weekly_donation_quota": 3,
		"weekly_donation_used":  0,
		"quota_reset_date":      time.Now().Format("2006-01-02"),
		"created_at": time.Now().Format(time.RFC3339),
		"updated_at": time.Now().Format(time.RFC3339),
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

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
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
		return nil, appError.New(appError.ErrDatabase, "Failed to create user in database")
	}

	// Parse created user
	var createdUsers []models.User
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &createdUsers); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse created user")
	}

	if len(createdUsers) == 0 {
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