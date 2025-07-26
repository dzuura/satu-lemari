package user

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

	"firebase.google.com/go/v4/auth"
	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type UserService struct {
	config        *config.Config
	fileUploadSvc *common.FileUploadService
	firebaseAuth  *auth.Client
	cache         *cache.RedisCache
}

func NewUserService(cfg *config.Config, firebaseAuth *auth.Client, redisCache *cache.RedisCache) *UserService {
	return &UserService{
		config:        cfg,
		fileUploadSvc: common.NewFileUploadService(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey),
		firebaseAuth:  firebaseAuth,
		cache:         redisCache,
	}
}

func (s *UserService) RegisterRoutes(r *mux.Router) {
	// Protected routes - require authentication
	r.HandleFunc("/users/me", s.GetMyProfile).Methods("GET")
	r.HandleFunc("/users/me", s.UpdateMyProfile).Methods("PUT")
	r.HandleFunc("/users/me", s.DeleteMyAccount).Methods("DELETE")
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
		recentRequests, err := s.getRecentRequestsSimple(userID, 5)
		if err != nil {
			log.Printf("Error getting recent requests: %v", err)
			recentRequests = []models.Request{}
		}

		recentItems, err := s.getRecentItems(userID, 5)
		if err != nil {
			log.Printf("Error getting recent items: %v", err)
			recentItems = []models.Item{}
		}

		// Create ordered response structure
		type PartnerDashboard struct {
			Stats          models.UserStats `json:"stats"`
			RecentRequests []models.Request `json:"recent_requests"`
			RecentItems    []models.Item    `json:"recent_items"`
		}

		dashboard = PartnerDashboard{
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
	// Try to get from cache first
	ctx := context.Background()
	cacheKey := s.cache.GenerateKey(cache.KeyUserProfile, userID)

	var cachedUser models.User
	if err := s.cache.Get(ctx, cacheKey, &cachedUser); err == nil {
		log.Printf("User %s retrieved from cache", userID)
		return &cachedUser, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=*", s.config.SupabaseURL, userID)
	log.Printf("Getting user by ID from database: %s", userID)
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

	user := &users[0]

	// Cache the user data for 15 minutes
	if err := s.cache.Set(ctx, cacheKey, user, 15*time.Minute); err != nil {
		log.Printf("Failed to cache user %s: %v", userID, err)
	} else {
		log.Printf("User %s cached successfully", userID)
	}

	return user, nil
}

func (s *UserService) getUserStats(userID, role string) (*models.UserStats, *appError.AppError) {
	// Try to get from cache first
	ctx := context.Background()
	cacheKey := s.cache.GenerateKey(cache.KeyUserStats, userID, role)

	var cachedStats models.UserStats
	if err := s.cache.Get(ctx, cacheKey, &cachedStats); err == nil {
		log.Printf("User stats for %s retrieved from cache", userID)
		return &cachedStats, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}

	stats := &models.UserStats{}

	if role == "partner" {
		// Partner Statistics

		// Active items count
		itemsURL := fmt.Sprintf("%s/rest/v1/items?partner_id=eq.%s&status=eq.active&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, itemsURL); err == nil {
			stats.ActiveItems = count
		}

		// Pending requests count (requests from users to this partner)
		pendingURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&status=eq.pending&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, pendingURL); err == nil {
			stats.PendingRequests = count
		}

		// Total donations completed (type=donation, status=completed)
		donationsURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&type=eq.donation&status=eq.completed&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, donationsURL); err == nil {
			stats.TotalDonations = count
		}

		// Total rentals completed (type=rental, status=completed)
		rentalsURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&type=eq.rental&status=eq.completed&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, rentalsURL); err == nil {
			stats.TotalRentals = count
		}

		// Completed requests = Total donations + Total rentals
		stats.CompletedRequests = stats.TotalDonations + stats.TotalRentals

		// Weekly quota fields are always 0 for partners
		stats.WeeklyQuotaUsed = 0
		stats.WeeklyQuotaRemaining = 0

	} else {
		// User Statistics

		// Active items is always 0 for regular users
		stats.ActiveItems = 0

		// Pending requests count (requests made by this user)
		pendingURL := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&status=eq.pending&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, pendingURL); err == nil {
			stats.PendingRequests = count
		}

		// Total donations received (type=donation, status=completed)
		donationsURL := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&type=eq.donation&status=eq.completed&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, donationsURL); err == nil {
			stats.TotalDonations = count
		}

		// Total rentals received (type=rental, status=completed)
		rentalsURL := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&type=eq.rental&status=eq.completed&select=count", s.config.SupabaseURL, userID)
		if count, err := s.getCount(client, rentalsURL); err == nil {
			stats.TotalRentals = count
		}

		// Completed requests = Total donations + Total rentals
		stats.CompletedRequests = stats.TotalDonations + stats.TotalRentals

		// Get user info for weekly quota
		user, err := s.getUserByID(userID)
		if err == nil {
			stats.WeeklyQuotaUsed = user.WeeklyDonationUsed
			stats.WeeklyQuotaRemaining = user.GetRemainingDonationQuota()
		}
	}

	// Cache the stats for 5 minutes (stats change frequently)
	if err := s.cache.Set(ctx, cacheKey, stats, 5*time.Minute); err != nil {
		log.Printf("Failed to cache user stats for %s: %v", userID, err)
	} else {
		log.Printf("User stats for %s cached successfully", userID)
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

func (s *UserService) getRecentRequestsSimple(userID string, limit int) ([]models.Request, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Get recent requests (all statuses) with item name
	url := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s&order=created_at.desc&limit=%d&select=*,items(name)",
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

	// Parse response with nested item data
	var rawRequests []map[string]interface{}
	if err := json.Unmarshal(body, &rawRequests); err != nil {
		return nil, err
	}

	var requests []models.Request
	for _, rawRequest := range rawRequests {
		// Convert to Request struct
		requestJSON, _ := json.Marshal(rawRequest)
		var request models.Request
		if err := json.Unmarshal(requestJSON, &request); err != nil {
			continue
		}

		// Extract item name from nested items data
		if items, ok := rawRequest["items"].(map[string]interface{}); ok {
			if itemName, ok := items["name"].(string); ok {
				request.ItemName = itemName
			}
		}

		// Populate user information for each request
		if userInfo, err := s.getUserInfoByID(request.UserID); err == nil {
			request.UserName = userInfo.Username
			request.UserFullName = userInfo.FullName
			request.UserPhone = userInfo.Phone
			request.UserPhoto = userInfo.Photo
		}

		requests = append(requests, request)
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

// UserInfo holds user information for dashboard
type UserInfo struct {
	Username string  `json:"username"`
	FullName string  `json:"full_name"`
	Phone    *string `json:"phone"`
	Photo    *string `json:"photo"`
}

// getUserInfoByID retrieves user info by ID from Supabase
func (s *UserService) getUserInfoByID(userID string) (*UserInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=username,full_name,phone,photo&limit=1",
		s.config.SupabaseURL, userID)

	log.Printf("Getting user info with URL: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

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

	log.Printf("User info response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d, body: %s", resp.StatusCode, string(body))
	}

	var users []UserInfo
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (s *UserService) validateUpdateProfileRequest(req *models.UpdateUserProfileRequest) *appError.AppError {
	// Validate username if provided
	if req.Username != nil && *req.Username != "" {
		if len(*req.Username) < 3 || len(*req.Username) > 30 {
			return appError.New(appError.ErrInvalidInput, "Username must be between 3 and 30 characters")
		}
	}

	// Validate full name if provided
	if req.FullName != nil && *req.FullName != "" {
		if len(*req.FullName) < 2 || len(*req.FullName) > 100 {
			return appError.New(appError.ErrInvalidInput, "Full name must be between 2 and 100 characters")
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

// DeleteMyAccount handles DELETE /users/me - permanently delete user account
func (s *UserService) DeleteMyAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Get user info to determine role
	user, appErr := s.getUserByID(userID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Delete user account based on role
	if user.Role == "partner" {
		appErr = s.deletePartnerAccount(userID)
	} else {
		appErr = s.deleteUserAccount(userID)
	}

	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Delete from Firebase Auth
	ctx := context.Background()
	err := s.firebaseAuth.DeleteUser(ctx, userID)
	if err != nil {
		log.Printf("Failed to delete user from Firebase: %v", err)
		// Continue anyway since Supabase deletion was successful
	}

	common.WriteSuccessResponse(w, nil, "Account deleted successfully")
}

// deletePartnerAccount deletes partner account with specific logic
func (s *UserService) deletePartnerAccount(userID string) *appError.AppError {
	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Soft delete requests where this partner is involved
	// Set deleted_by_partner = true for all requests where partner_id = userID
	requestUpdateURL := fmt.Sprintf("%s/rest/v1/requests?partner_id=eq.%s", s.config.SupabaseURL, userID)
	requestUpdateData := map[string]interface{}{
		"deleted_by_partner": true,
		"updated_at":         time.Now().Format(time.RFC3339),
	}

	requestUpdateJSON, err := json.Marshal(requestUpdateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal request update data")
	}

	requestUpdateReq, err := http.NewRequest("PATCH", requestUpdateURL, bytes.NewBuffer(requestUpdateJSON))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request update request")
	}

	requestUpdateReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	requestUpdateReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	requestUpdateReq.Header.Set("Content-Type", "application/json")

	requestUpdateResp, err := client.Do(requestUpdateReq)
	if err != nil {
		log.Printf("Failed to soft delete partner requests: %v", err)
		// Continue with deletion process
	} else {
		requestUpdateResp.Body.Close()
	}

	// Step 2: Delete all items owned by this partner (CASCADE will handle related requests/transactions)
	itemDeleteURL := fmt.Sprintf("%s/rest/v1/items?partner_id=eq.%s", s.config.SupabaseURL, userID)
	itemDeleteReq, err := http.NewRequest("DELETE", itemDeleteURL, nil)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create item delete request")
	}

	itemDeleteReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	itemDeleteReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	itemDeleteResp, err := client.Do(itemDeleteReq)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to delete partner items")
	}
	itemDeleteResp.Body.Close()

	// Step 3: Anonymize partner data instead of deleting to preserve request history
	// Update partner record to anonymized state
	userUpdateURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	anonymizedData := map[string]interface{}{
		"email":                 fmt.Sprintf("deleted-partner-%s@deleted.local", userID[:8]),
		"username":              fmt.Sprintf("deleted-partner-%s", userID[:8]),
		"full_name":             "[Deleted Partner]",
		"phone":                 nil,
		"address":               nil,
		"city":                  nil,
		"latitude":              nil,
		"longitude":             nil,
		"photo":                 nil,
		"description":           nil,
		"is_active":             false,
		"weekly_donation_quota": 0,
		"weekly_donation_used":  0,
		"updated_at":            time.Now().Format(time.RFC3339),
	}

	anonymizedJSON, err := json.Marshal(anonymizedData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal anonymized partner data")
	}

	userUpdateReq, err := http.NewRequest("PATCH", userUpdateURL, bytes.NewBuffer(anonymizedJSON))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create partner update request")
	}

	userUpdateReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	userUpdateReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	userUpdateReq.Header.Set("Content-Type", "application/json")

	userUpdateResp, err := client.Do(userUpdateReq)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to anonymize partner account")
	}
	userUpdateResp.Body.Close()

	log.Printf("Partner account anonymized successfully: %s", userID)
	return nil
}

// deleteUserAccount deletes regular user account with specific logic
func (s *UserService) deleteUserAccount(userID string) *appError.AppError {
	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Soft delete requests where this user is involved
	// Set deleted_by_user = true for all requests where user_id = userID
	requestUpdateURL := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s", s.config.SupabaseURL, userID)
	requestUpdateData := map[string]interface{}{
		"deleted_by_user": true,
		"updated_at":      time.Now().Format(time.RFC3339),
	}

	requestUpdateJSON, err := json.Marshal(requestUpdateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal request update data")
	}

	requestUpdateReq, err := http.NewRequest("PATCH", requestUpdateURL, bytes.NewBuffer(requestUpdateJSON))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request update request")
	}

	requestUpdateReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	requestUpdateReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	requestUpdateReq.Header.Set("Content-Type", "application/json")

	requestUpdateResp, err := client.Do(requestUpdateReq)
	if err != nil {
		log.Printf("Failed to soft delete user requests: %v", err)
		// Continue with deletion process
	} else {
		requestUpdateResp.Body.Close()
	}

	// Step 2: Anonymize user data instead of deleting to preserve request history
	// Update user record to anonymized state
	userUpdateURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	anonymizedData := map[string]interface{}{
		"email":                 fmt.Sprintf("deleted-user-%s@deleted.local", userID[:8]),
		"username":              fmt.Sprintf("deleted-user-%s", userID[:8]),
		"full_name":             "[Deleted User]",
		"phone":                 nil,
		"address":               nil,
		"city":                  nil,
		"latitude":              nil,
		"longitude":             nil,
		"photo":                 nil,
		"description":           nil,
		"is_active":             false,
		"weekly_donation_quota": 0,
		"weekly_donation_used":  0,
		"updated_at":            time.Now().Format(time.RFC3339),
	}

	anonymizedJSON, err := json.Marshal(anonymizedData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal anonymized user data")
	}

	userUpdateReq, err := http.NewRequest("PATCH", userUpdateURL, bytes.NewBuffer(anonymizedJSON))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create user update request")
	}

	userUpdateReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	userUpdateReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	userUpdateReq.Header.Set("Content-Type", "application/json")

	userUpdateResp, err := client.Do(userUpdateReq)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to anonymize user account")
	}
	userUpdateResp.Body.Close()

	log.Printf("User account anonymized successfully: %s", userID)
	return nil
}
