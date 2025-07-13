package user

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type UserService struct {
	config        *config.Config
	fileUploadSvc *common.FileUploadService
}

func NewUserService(cfg *config.Config) *UserService {
	return &UserService{
		config:        cfg,
		fileUploadSvc: common.NewFileUploadService(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey),
	}
}

func (s *UserService) RegisterRoutes(r *mux.Router) {
	// Protected routes - require authentication
	r.HandleFunc("/users/me", s.GetMyProfile).Methods("GET")
	r.HandleFunc("/users/me", s.UpdateMyProfile).Methods("PUT")
	r.HandleFunc("/users/dashboard", s.GetDashboard).Methods("GET")

	// Public routes
	r.HandleFunc("/users/{user_id}/profile", s.GetUserProfile).Methods("GET")
	r.HandleFunc("/users/search", s.SearchUsers).Methods("GET")
}

func (s *UserService) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	log.Printf("GetMyProfile called with userID: %s", userID)

	user, err := s.getUserByID(userID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, user, "Profile retrieved successfully")
}

func (s *UserService) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Parse multipart form (max 10MB)
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Failed to parse form data"),
			common.GenerateTraceID())
		return
	}

	// Create update request from form data
	req := &models.UpdateUserProfileRequest{}

	// Extract form fields directly
	if username := r.FormValue("username"); username != "" {
		req.Username = &username
	}
	if fullName := r.FormValue("full_name"); fullName != "" {
		req.FullName = &fullName
	}
	if phone := r.FormValue("phone"); phone != "" {
		req.Phone = &phone
	}
	if address := r.FormValue("address"); address != "" {
		req.Address = &address
	}
	if city := r.FormValue("city"); city != "" {
		req.City = &city
	}
	if description := r.FormValue("description"); description != "" {
		req.Description = &description
	}

	// Handle numeric fields
	if latitudeStr := r.FormValue("latitude"); latitudeStr != "" {
		log.Printf("Parsing latitude: %s", latitudeStr)
		if latitude, err := common.ParseAndValidateLatitude(latitudeStr); err == nil {
			req.Latitude = &latitude
			log.Printf("Successfully parsed latitude: %f", latitude)
		} else {
			log.Printf("Failed to parse latitude '%s': %v", latitudeStr, err)
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid latitude: %v", err)),
				common.GenerateTraceID())
			return
		}
	}
	if longitudeStr := r.FormValue("longitude"); longitudeStr != "" {
		log.Printf("Parsing longitude: %s", longitudeStr)
		if longitude, err := common.ParseAndValidateLongitude(longitudeStr); err == nil {
			req.Longitude = &longitude
			log.Printf("Successfully parsed longitude: %f", longitude)
		} else {
			log.Printf("Failed to parse longitude '%s': %v", longitudeStr, err)
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid longitude: %v", err)),
				common.GenerateTraceID())
			return
		}
	}

	// Validate input
	if err := s.validateUpdateProfileRequest(req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Handle photo upload if provided
	var photoURL *string
	if file, header, err := r.FormFile("photo"); err == nil {
		defer file.Close()

		// Validate image file
		if err := common.ValidateImageFile(header); err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, err.Error()),
				common.GenerateTraceID())
			return
		}

		// Upload photo to Supabase Storage
		uploadedURL, err := s.fileUploadSvc.UploadUserPhoto(userID, header)
		if err != nil {
			log.Printf("Failed to upload photo: %v", err)
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInternal, "Failed to upload photo"),
				common.GenerateTraceID())
			return
		}

		photoURL = &uploadedURL
	}

	// Update user in database
	updatedUser, appErr := s.updateUserProfile(userID, req, photoURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedUser, "Profile updated successfully")
}

func (s *UserService) GetDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	role, ok := common.GetUserRoleFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Get user stats
	stats, err := s.getUserStats(userID, role)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Get recent activities based on role
	var dashboard interface{}
	if role == "partner" {
		recentRequests, err := s.getRecentRequests(userID, 5)
		if err != nil {
			log.Printf("Error getting recent requests: %v", err)
			recentRequests = []models.Request{}
		}

		recentItems, err := s.getRecentItems(userID, 5)
		if err != nil {
			log.Printf("Error getting recent items: %v", err)
			recentItems = []models.Item{}
		}

		dashboard = models.UserDashboard{
			Stats:          *stats,
			RecentRequests: recentRequests,
			RecentItems:    recentItems,
		}
	} else {
		// For regular users, just return stats
		dashboard = map[string]interface{}{
			"stats": stats,
		}
	}

	common.WriteSuccessResponse(w, dashboard, "Dashboard data retrieved successfully")
}

func (s *UserService) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	userID := common.GetPathParam(r, "user_id")

	// Validate user ID format (Firebase UID format)
	if userID == "" || len(userID) < 10 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid user ID"),
			common.GenerateTraceID())
		return
	}

	user, appErr := s.getUserByID(userID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Return public profile only
	profile := user.ToProfile()
	common.WriteSuccessResponse(w, profile, "User profile retrieved successfully")
}

func (s *UserService) SearchUsers(w http.ResponseWriter, r *http.Request) {
	search := common.GetQueryParam(r, "q", "")
	role := common.GetQueryParam(r, "role", "")
	city := common.GetQueryParam(r, "city", "")
	pagination := common.GetPaginationParams(r)

	users, total, err := s.searchUsers(search, role, city, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, users, meta, "Users retrieved successfully")
}

// Helper methods

func (s *UserService) getUserByID(userID string) (*models.User, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=*", s.config.SupabaseURL, userID)
	log.Printf("Getting user by ID: %s", userID)
	log.Printf("URL: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	// Use service role key to bypass RLS for user queries
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Database query failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Database query failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	log.Printf("Database response: %s", string(body))

	var users []models.User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	log.Printf("Found %d users", len(users))

	if len(users) == 0 {
		return nil, appError.ErrUserNotFound
	}

	return &users[0], nil
}

func (s *UserService) getUserStats(userID, role string) (*models.UserStats, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	stats := &models.UserStats{}

	if role == "partner" {
		// Get partner statistics
		// Active items count
		itemsURL := fmt.Sprintf("%s/rest/v1/items?partner_id=eq.%s&status=eq.active&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, itemsURL); err == nil {
			stats.ActiveItems = count
		}

		// Pending requests count
		requestsURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&status=eq.pending&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, requestsURL); err == nil {
			stats.PendingRequests = count
		}

		// Completed requests count
		completedURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&status=eq.completed&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, completedURL); err == nil {
			stats.CompletedRequests = count
		}
	} else {
		// Get user statistics
		// Total requests count
		requestsURL := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, requestsURL); err == nil {
			stats.TotalDonations = count // This includes both donations and rentals
		}

		// Get user info for quota
		user, err := s.getUserByID(userID)
		if err == nil {
			stats.WeeklyQuotaUsed = user.WeeklyDonationUsed
			stats.WeeklyQuotaRemaining = user.GetRemainingDonationQuota()
		}
	}

	return stats, nil
}

func (s *UserService) getCount(client *http.Client, url string) (int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	// Use service role key to bypass RLS for count queries
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if len(result) > 0 {
		if count, ok := result[0]["count"].(float64); ok {
			return int(count), nil
		}
	}

	return 0, nil
}

func (s *UserService) getRecentRequests(userID string, limit int) ([]models.Request, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&order=created_at.desc&limit=%d&select=*",
		s.config.SupabaseURL, userID, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var requests []models.Request
	if err := json.Unmarshal(body, &requests); err != nil {
		return nil, err
	}

	return requests, nil
}

func (s *UserService) getRecentItems(userID string, limit int) ([]models.Item, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/items?partner_id=eq.%s&order=created_at.desc&limit=%d&select=*",
		s.config.SupabaseURL, userID, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []models.Item
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *UserService) searchUsers(search, role, city string, pagination common.PaginationParams) ([]models.UserProfile, int, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Build base URL
	baseURL := fmt.Sprintf("%s/rest/v1/users", s.config.SupabaseURL)

	// Build query parameters
	params := []string{"select=id,username,full_name,photo,city,description,role,created_at"}

	// Add search filter
	if search != "" {
		// Use ilike for case-insensitive partial matching
		searchFilter := fmt.Sprintf("or=(username.ilike.*%s*,full_name.ilike.*%s*)", search, search)
		params = append(params, searchFilter)
	}

	// Add role filter
	if role != "" {
		roleFilter := fmt.Sprintf("role=eq.%s", role)
		params = append(params, roleFilter)
	}

	// Add city filter
	if city != "" {
		cityFilter := fmt.Sprintf("city.ilike.*%s*", city)
		params = append(params, cityFilter)
	}

	// Add pagination
	offset := (pagination.Page - 1) * pagination.Limit
	params = append(params, fmt.Sprintf("limit=%d", pagination.Limit))
	params = append(params, fmt.Sprintf("offset=%d", offset))

	// Build final URL
	queryString := strings.Join(params, "&")
	url := fmt.Sprintf("%s?%s", baseURL, queryString)

	log.Printf("Search URL: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to create request")
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Search failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return nil, 0, appError.New(appError.ErrDatabase, "Database query failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to read response")
	}

	log.Printf("Search response: %s", string(body))

	var users []models.UserProfile
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	// Get total count for pagination
	// Build count query without pagination
	countParams := []string{"select=count"}
	if search != "" {
		countParams = append(countParams, fmt.Sprintf("or=(username.ilike.*%s*,full_name.ilike.*%s*)", search, search))
	}
	if role != "" {
		countParams = append(countParams, fmt.Sprintf("role=eq.%s", role))
	}
	if city != "" {
		countParams = append(countParams, fmt.Sprintf("city.ilike.*%s*", city))
	}

	countQueryString := strings.Join(countParams, "&")
	countURL := fmt.Sprintf("%s?%s", baseURL, countQueryString)

	countReq, err := http.NewRequest("GET", countURL, nil)
	if err != nil {
		log.Printf("Failed to create count request: %v", err)
		total := len(users) // Fallback
		return users, total, nil
	}

	// Use service role key to bypass RLS
	countReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	countReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	countResp, err := client.Do(countReq)
	if err != nil {
		log.Printf("Failed to get count: %v", err)
		total := len(users) // Fallback
		return users, total, nil
	}
	defer countResp.Body.Close()

	var countResult []map[string]interface{}
	countBody, _ := io.ReadAll(countResp.Body)
	if err := json.Unmarshal(countBody, &countResult); err != nil {
		log.Printf("Failed to parse count response: %v", err)
		total := len(users) // Fallback
		return users, total, nil
	}

	total := 0
	if len(countResult) > 0 {
		if count, ok := countResult[0]["count"].(float64); ok {
			total = int(count)
		}
	}

	return users, total, nil
}

func (s *UserService) validateUpdateProfileRequest(req *models.UpdateUserProfileRequest) *appError.AppError {
	// Validate username if provided
	if req.Username != nil && *req.Username != "" {
		if len(*req.Username) < 3 || len(*req.Username) > 30 {
			return appError.New(appError.ErrInvalidInput, "Username must be between 3 and 30 characters")
		}
	}

	// Validate phone number if provided
	if req.Phone != nil && *req.Phone != "" {
		if !common.IsValidPhoneNumber(*req.Phone) {
			return appError.New(appError.ErrInvalidInput, "Invalid phone number format")
		}
	}

	// Validate latitude if provided
	if req.Latitude != nil {
		if err := common.ValidateLatitude(*req.Latitude); err != nil {
			return appError.New(appError.ErrInvalidInput, err.Error())
		}
	}

	// Validate longitude if provided
	if req.Longitude != nil {
		if err := common.ValidateLongitude(*req.Longitude); err != nil {
			return appError.New(appError.ErrInvalidInput, err.Error())
		}
	}

	return nil
}

func (s *UserService) updateUserProfile(userID string, req *models.UpdateUserProfileRequest, photoURL *string) (*models.User, *appError.AppError) {
	updateData := map[string]interface{}{
		"updated_at": time.Now().Format(time.RFC3339),
	}

	// Only include non-nil fields
	if req.Username != nil {
		updateData["username"] = *req.Username
	}
	if req.FullName != nil {
		updateData["full_name"] = *req.FullName
	}
	if req.Phone != nil {
		updateData["phone"] = *req.Phone
	}
	if req.Address != nil {
		updateData["address"] = *req.Address
	}
	if req.City != nil {
		updateData["city"] = *req.City
	}
	if req.Latitude != nil {
		updateData["latitude"] = *req.Latitude
	}
	if req.Longitude != nil {
		updateData["longitude"] = *req.Longitude
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if photoURL != nil {
		updateData["photo"] = *photoURL
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	httpReq, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	// Use service role key to bypass RLS for user updates
	httpReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to update user")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to update user: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Failed to update user in database")
	}

	// Parse updated user
	var updatedUsers []models.User
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &updatedUsers); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse updated user")
	}

	if len(updatedUsers) == 0 {
		return nil, appError.New(appError.ErrInternal, "No user returned from update")
	}

	return &updatedUsers[0], nil
}
