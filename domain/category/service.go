package category

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

// CategoryService handles category operations
type CategoryService struct {
	config *config.Config
}

// NewCategoryService creates a new category service
func NewCategoryService(cfg *config.Config) *CategoryService {
	return &CategoryService{
		config: cfg,
	}
}

// RegisterRoutes registers category routes
func (s *CategoryService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/categories", s.GetCategories).Methods("GET")
	r.HandleFunc("/categories/{id}", s.GetCategory).Methods("GET")
	r.HandleFunc("/categories", s.CreateCategory).Methods("POST")
	r.HandleFunc("/categories/{id}", s.UpdateCategory).Methods("PUT")
	r.HandleFunc("/categories/{id}", s.DeleteCategory).Methods("DELETE")
}

// GetCategories handles GET /categories
func (s *CategoryService) GetCategories(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	search := r.URL.Query().Get("search")
	isActive := r.URL.Query().Get("is_active")

	// Parse pagination
	pagination := common.GetPaginationParams(r)

	// Get categories
	categories, total, err := s.searchCategories(search, isActive, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Write response
	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, categories, meta, "Categories retrieved successfully")
}

// GetCategory handles GET /categories/{id}
func (s *CategoryService) GetCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	categoryID := vars["id"]

	if categoryID == "" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Category ID is required"), common.GenerateTraceID())
		return
	}

	// Get category
	category, err := s.getCategoryByID(categoryID)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Write response
	common.WriteSuccessResponse(w, category, "Category retrieved successfully")
}

// CreateCategory handles POST /categories
func (s *CategoryService) CreateCategory(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req models.CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid request body"), common.GenerateTraceID())
		return
	}

	// Validate request
	if err := s.validateCreateRequest(&req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Check if category name already exists
	exists, err := s.categoryNameExists(req.Name)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}
	if exists {
		appError.WriteErrorResponse(w, appError.New(appError.ErrConflict, "Category name already exists"), common.GenerateTraceID())
		return
	}

	// Create category
	category, err := s.createCategory(&req)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Write response
	common.WriteSuccessResponse(w, category, "Category created successfully")
}

// UpdateCategory handles PUT /categories/{id}
func (s *CategoryService) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	categoryID := vars["id"]

	if categoryID == "" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Category ID is required"), common.GenerateTraceID())
		return
	}

	// Parse request body
	var req models.UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid request body"), common.GenerateTraceID())
		return
	}

	// Validate request
	if err := s.validateUpdateRequest(&req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Check if category exists
	existingCategory, err := s.getCategoryByID(categoryID)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Check if new name conflicts with existing category
	if req.Name != nil && *req.Name != existingCategory.Name {
		exists, err := s.categoryNameExists(*req.Name)
		if err != nil {
			appError.WriteErrorResponse(w, err, common.GenerateTraceID())
			return
		}
		if exists {
			appError.WriteErrorResponse(w, appError.New(appError.ErrConflict, "Category name already exists"), common.GenerateTraceID())
			return
		}
	}

	// Update category
	category, err := s.updateCategory(categoryID, &req)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Write response
	common.WriteSuccessResponse(w, category, "Category updated successfully")
}

// DeleteCategory handles DELETE /categories/{id}
func (s *CategoryService) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	categoryID := vars["id"]

	if categoryID == "" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Category ID is required"), common.GenerateTraceID())
		return
	}

	// Check if category has items
	hasItems, err := s.categoryHasItems(categoryID)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}
	if hasItems {
		appError.WriteErrorResponse(w, appError.New(appError.ErrConflict, "Cannot delete category with existing items"), common.GenerateTraceID())
		return
	}

	// Delete category (soft delete)
	err = s.deleteCategory(categoryID)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Write response
	common.WriteSuccessResponse(w, map[string]string{"message": "Category deleted successfully"}, "Category deleted successfully")
}

// searchCategories searches categories with filters and pagination
func (s *CategoryService) searchCategories(search, isActive string, pagination common.PaginationParams) ([]models.Category, int, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Build query
	query := "select=*"

	// Add filters
	var filters []string

	if search != "" {
		filters = append(filters, fmt.Sprintf("name.ilike.%%%s%%", search))
	}

	if isActive != "" {
		switch isActive {
		case "true":
			filters = append(filters, "is_active.eq.true")
		case "false":
			filters = append(filters, "is_active.eq.false")
		}
	}

	// Add filters to query
	for _, filter := range filters {
		query += "&" + filter
	}

	// Add sorting
	query += "&order=name.asc"

	// Add pagination
	offset := (pagination.Page - 1) * pagination.Limit
	query += fmt.Sprintf("&limit=%d&offset=%d", pagination.Limit, offset)

	url := fmt.Sprintf("%s/rest/v1/categories?%s", s.config.SupabaseURL, query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to read response")
	}

	var categories []models.Category
	if err := json.Unmarshal(body, &categories); err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	// Get total count
	countURL := fmt.Sprintf("%s/rest/v1/categories?select=count", s.config.SupabaseURL)
	if len(filters) > 0 {
		for _, filter := range filters {
			countURL += "&" + filter
		}
	}

	total, _ := s.getCount(client, countURL)

	return categories, total, nil
}

func (s *CategoryService) getCategoryByID(categoryID string) (*models.Category, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/categories?id=eq.%s&select=*", s.config.SupabaseURL, categoryID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	var categories []models.Category
	if err := json.Unmarshal(body, &categories); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	if len(categories) == 0 {
		return nil, appError.ErrCategoryNotFound
	}

	return &categories[0], nil
}

func (s *CategoryService) createCategory(req *models.CreateCategoryRequest) (*models.Category, *appError.AppError) {
	newCategory := map[string]interface{}{
		"id":          uuid.New().String(),
		"name":        req.Name,
		"description": req.Description,
		"icon":        req.Icon,
		"is_active":   true,
		"created_at":  time.Now().Format(time.RFC3339),
		"updated_at":  time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(newCategory)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal category data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/categories", s.config.SupabaseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	httpReq.Header.Set("apikey", s.config.SupabaseKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to create category")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to create category: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Failed to create category in database")
	}

	// Parse created category
	var createdCategories []models.Category
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &createdCategories); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse created category")
	}

	if len(createdCategories) == 0 {
		return nil, appError.New(appError.ErrInternal, "No category returned from creation")
	}

	return &createdCategories[0], nil
}

func (s *CategoryService) updateCategory(categoryID string, req *models.UpdateCategoryRequest) (*models.Category, *appError.AppError) {
	updateData := map[string]interface{}{
		"updated_at": time.Now().Format(time.RFC3339),
	}

	// Only include non-nil fields
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if req.Icon != nil {
		updateData["icon"] = *req.Icon
	}
	if req.IsActive != nil {
		updateData["is_active"] = *req.IsActive
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/categories?id=eq.%s", s.config.SupabaseURL, categoryID)
	httpReq, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create request")
	}

	httpReq.Header.Set("apikey", s.config.SupabaseKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to update category")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to update category: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Failed to update category in database")
	}

	// Parse updated category
	var updatedCategories []models.Category
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &updatedCategories); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse updated category")
	}

	if len(updatedCategories) == 0 {
		return nil, appError.New(appError.ErrInternal, "No category returned from update")
	}

	return &updatedCategories[0], nil
}

func (s *CategoryService) deleteCategory(categoryID string) *appError.AppError {
	updateData := map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal delete data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/categories?id=eq.%s", s.config.SupabaseURL, categoryID)
	httpReq, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request")
	}

	httpReq.Header.Set("apikey", s.config.SupabaseKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to delete category")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to delete category: %s", string(bodyBytes))
		return appError.New(appError.ErrDatabase, "Failed to delete category in database")
	}

	return nil
}

func (s *CategoryService) categoryNameExists(name string) (bool, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/categories?name=eq.%s&select=id", s.config.SupabaseURL, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to read response")
	}

	var categories []map[string]interface{}
	if err := json.Unmarshal(body, &categories); err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	return len(categories) > 0, nil
}

func (s *CategoryService) categoryHasItems(categoryID string) (bool, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/items?category_id=eq.%s&select=id&limit=1", s.config.SupabaseURL, categoryID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to read response")
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	return len(items) > 0, nil
}

func (s *CategoryService) getCount(client *http.Client, url string) (int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Parse count from response
	var countData []map[string]interface{}
	if err := json.Unmarshal(body, &countData); err != nil {
		return 0, err
	}

	if len(countData) > 0 {
		if count, ok := countData[0]["count"]; ok {
			if countFloat, ok := count.(float64); ok {
				return int(countFloat), nil
			}
		}
	}

	return 0, nil
}

func (s *CategoryService) validateCreateRequest(req *models.CreateCategoryRequest) *appError.AppError {
	if strings.TrimSpace(req.Name) == "" {
		return appError.New(appError.ErrInvalidInput, "Category name is required")
	}

	if len(req.Name) > 100 {
		return appError.New(appError.ErrInvalidInput, "Category name must be less than 100 characters")
	}

	if req.Description != nil && len(*req.Description) > 500 {
		return appError.New(appError.ErrInvalidInput, "Category description must be less than 500 characters")
	}

	return nil
}

func (s *CategoryService) validateUpdateRequest(req *models.UpdateCategoryRequest) *appError.AppError {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return appError.New(appError.ErrInvalidInput, "Category name cannot be empty")
	}

	if req.Name != nil && len(*req.Name) > 100 {
		return appError.New(appError.ErrInvalidInput, "Category name must be less than 100 characters")
	}

	if req.Description != nil && len(*req.Description) > 500 {
		return appError.New(appError.ErrInvalidInput, "Category description must be less than 500 characters")
	}

	return nil
}
