package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type AuthService struct {
	config *config.Config
	client *auth.Client
}

func NewAuthService(cfg *config.Config) *AuthService {
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

	// Marshal to JSON
	credJSON, err := json.Marshal(credMap)
	if err != nil {
		log.Fatalf("error marshaling credentials: %v", err)
	}

	// Initialize Firebase app with credentials
	app, err := firebase.NewApp(context.Background(), &firebase.Config{
		ProjectID: cfg.FirebaseProjectID,
	}, option.WithCredentialsJSON(credJSON))
	if err != nil {
		log.Fatalf("error initializing app: %v", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("error getting Auth client: %v", err)
	}
	return &AuthService{config: cfg, client: client}
}

func (s *AuthService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/auth/verify", s.VerifyAuth).Methods("POST")
}

func (s *AuthService) VerifyAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Token    string `json:"token"`
		Type     string `json:"type"`
		Platform string `json:"platform"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := context.Background()
	var user *auth.UserRecord
	var uid string

	switch req.Type {
	case "firebase":
		// Verify Firebase ID Token
		token, err := s.client.VerifyIDToken(ctx, req.Token)
		if err != nil {
			sendErrorResponse(w, http.StatusUnauthorized, "Invalid Firebase token")
			return
		}
		user, err = s.client.GetUser(ctx, token.UID)
		if err != nil {
			log.Printf("Error getting user: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		uid = token.UID
	case "google":
		// Verify Google ID Token
		token, err := s.client.VerifyIDToken(ctx, req.Token)
		if err != nil {
			sendErrorResponse(w, http.StatusUnauthorized, "Invalid Google token")
			return
		}
		user, err = s.client.GetUser(ctx, token.UID)
		if err != nil {
			if auth.IsUserNotFound(err) {
				// Create new user if not found
				user, err = s.client.CreateUser(ctx, (&auth.UserToCreate{}).
					UID(token.UID).
					Email(token.Claims["email"].(string)).
					DisplayName(token.Claims["name"].(string)))
				if err != nil {
					log.Printf("Error creating user: %v", err)
					sendErrorResponse(w, http.StatusInternalServerError, err.Error())
					return
				}
			} else {
				log.Printf("Error getting user: %v", err)
				sendErrorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		uid = token.UID
	default:
		sendErrorResponse(w, http.StatusBadRequest, "Invalid type, use 'firebase' or 'google'")
		return
	}

	// Determine default role based on platform
	var defaultRole string
	switch strings.ToLower(req.Platform) {
	case "web":
		defaultRole = "partner"
	case "mobile":
		defaultRole = "user"
	default:
		defaultRole = "user"
	}

	// Determine username
	var username string
	if req.Username != "" {
		username = req.Username
	} else if user.DisplayName != "" {
		username = user.DisplayName
	} else {
		// Fetch existing username from Supabase if available
		existingUser, err := s.getUserFromSupabase(uid) // Use UID as identifier
		if err == nil && existingUser.Username != "" {
			username = existingUser.Username
		} else {
			username = fmt.Sprintf("user_%s", uid[:8]) // Fallback default username
		}
	}

	// Create custom token
	customToken, err := s.client.CustomToken(ctx, uid)
	if err != nil {
		log.Printf("Error creating custom token: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync with Supabase
	s.syncUserWithSupabase(uid, user.Email, username, defaultRole)

	// Return response
	response := map[string]interface{}{
		"user": map[string]string{
			"email":    user.Email,
			"username": username,
			"role":     defaultRole,
		},
		"accessToken": customToken,
	}
	json.NewEncoder(w).Encode(response)
}

func sendErrorResponse(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *AuthService) syncUserWithSupabase(uid, email, username, role string) {
	client := &http.Client{}

	// Check if user exists
	checkURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=id,email,username,role", s.config.SupabaseURL, uid)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}

	req.Header.Add("apikey", s.config.SupabaseKey)
	req.Header.Add("Authorization", "Bearer "+s.config.SupabaseKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error checking user in Supabase: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return
	}

	var existingUsers []struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(body, &existingUsers); err != nil {
		log.Printf("Error parsing response: %v", err)
		return
	}

	if len(existingUsers) == 0 {
		// Create new user only if not exists
		newUser := map[string]interface{}{
			"id":         uid,
			"email":      email,
			"username":   username,
			"role":       role,
			"photo":      "",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		}

		jsonData, err := json.Marshal(newUser)
		if err != nil {
			log.Printf("Error marshaling new user: %v", err)
			return
		}
		log.Printf("Sending to Supabase: %s", string(jsonData)) // Log data yang dikirim

		insertURL := fmt.Sprintf("%s/rest/v1/users", s.config.SupabaseURL)
		req, err = http.NewRequest("POST", insertURL, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Error creating insert request: %v", err)
			return
		}

		req.Header.Add("apikey", s.config.SupabaseKey)
		req.Header.Add("Authorization", "Bearer "+s.config.SupabaseKey)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Prefer", "return=representation")

		resp, err = client.Do(req)
		if err != nil {
			log.Printf("Error inserting user: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			errorBody, _ := io.ReadAll(resp.Body)
			log.Printf("Error inserting user, status: %d, body: %s", resp.StatusCode, string(errorBody))
		}
	}
}

type SupabaseUser struct {
	Username string `json:"username"`
}

func (s *AuthService) getUserFromSupabase(uid string) (SupabaseUser, error) {
	client := &http.Client{}

	checkURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=username", s.config.SupabaseURL, uid)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		log.Printf("Error creating getUser request: %v", err)
		return SupabaseUser{}, err
	}

	req.Header.Add("apikey", s.config.SupabaseKey)
	req.Header.Add("Authorization", "Bearer "+s.config.SupabaseKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error fetching user from Supabase: %v", err)
		return SupabaseUser{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading getUser response: %v", err)
		return SupabaseUser{}, err
	}

	var users []SupabaseUser
	if err := json.Unmarshal(body, &users); err != nil {
		log.Printf("Error parsing getUser response: %v", err)
		return SupabaseUser{}, err
	}

	if len(users) > 0 {
		return users[0], nil
	}
	return SupabaseUser{}, fmt.Errorf("user not found")
}
