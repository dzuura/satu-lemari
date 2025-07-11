package category

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/google/uuid"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type CategoryService struct {
	config *config.Config
}

func NewCategoryService(cfg *config.Config) *CategoryService {
	return &CategoryService{
		config: cfg,
	}
}

func (s *CategoryService) RegisterRoutes(r *mux.Router) {
	// Public routes
	r.HandleFunc("/categories", s.GetCategories).Methods("GET")
	r.HandleFunc("/categories/{category_id}", s.GetCategory).Methods("GET")
	
	// Admin only routes
	r.HandleFunc("/categories", s.CreateCategory).Methods("POST")
	r.HandleFunc("/categories/{category_id}", s.UpdateCategory).Methods("PUT")
	r.HandleFunc("/categories/{category_id}", s.DeleteCategory).Methods("DELETE")
}

func (s *CategoryService) GetCategories(w http.ResponseWriter, r *http.Request) {
	search := common.GetQueryParam(r, "search", "")
	isActive := common.GetQueryParam(r, "is_active", "true")
	pagination := common.GetPaginationParams(r)

	categories, total, err := s.searchCategories(search, isActive, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, categories, meta, "Categories retrieved successfully")
}

func (s *CategoryService) GetCategory(w http.ResponseWriter, r *http.Request) {
	categoryIDStr := common.GetPathParam(r, "category_id")
	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid category ID"),
			common.GenerateTraceID())
		return
	}

	category, appErr := s.getCategoryByID(categoryID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, category, "Category retrieved successfully")
}

func (s *CategoryService) CreateCategory(w http.ResponseWriter, r *http.Request) {
	// Verify admin role
	role, ok := common.GetUserRoleFromContext(r)
	if !ok || role != "admin" {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	var req models.CreateCategoryRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
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
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrCategoryExists, "Category name already exists"),
			common.GenerateTraceID())
		return
	}

	// Create category
	category, err := s.createCategory(&req)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, category, "Category created successfully")
}

func (s *CategoryService) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	// Verify admin role
	role, ok := common.GetUserRoleFromContext(r)
	if !ok || role != "admin" {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	categoryIDStr := common.GetPathParam(r, "category_id")
	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid category ID"),
			common.GenerateTraceID())
		return
	}

	var req models.UpdateCategoryRequest
	if jsonErr := common.ParseJSONBody(r, &req); jsonErr != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate request
	if appErr := s.validateUpdateRequest(&req); appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check if category exists
	existing, appErr := s.getCategoryByID(categoryID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check if new name conflicts with another category
	if req.Name != nil && *req.Name != existing.Name {
		exists, appErr := s.categoryNameExists(*req.Name)
		if appErr != nil {
			appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
			return
		}
		if exists {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrCategoryExists, "Category name already exists"),
				common.GenerateTraceID())
			return
		}
	}

	// Update category
	updatedCategory, appErr := s.updateCategory(categoryID.String(), &req)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedCategory, "Category updated successfully")
}

func (s *CategoryService) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	// Verify admin role
	role, ok := common.GetUserRoleFromContext(r)
	if !ok || role != "admin" {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	categoryIDStr := common.GetPathParam(r, "category_id")
	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid category ID"),
			common.GenerateTraceID())
		return
	}

	// Check if category has associated items
	hasItems, appErr := s.categoryHasItems(categoryID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}
	if hasItems {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrCategoryInUse, "Cannot delete category with associated items"),
			common.GenerateTraceID())
		return
	}

	// Delete category (soft delete by setting is_active to false)
	appErr = s.deleteCategory(categoryID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, nil, "Category deleted successfully")
}

// Helper methods

func (s *CategoryService) searchCategories(search, isActive string, pagination common.PaginationParams) ([]models.Category, int, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Build query
	query := "select=id,name,description,icon,is_active,created_at,updated_at"
	filters := []string{}

	if search != "" {
		filters = append(filters, fmt.Sprintf("or=(name.ilike.%%%s%%,description.ilike.%%%s%%)", search, search))
	}
	if isActive != "" {
		filters = append(filters, fmt.Sprintf("is_active=eq.%s", isActive))
	}

	// Add filters to query
	for _, filter := range filters {
		query = "&"  filter
	}

	// Add sorting
	query = "&order=name.asc"

	// Add pagination
	offset := (pagination.Page - 1) * pagination.Limit
	query = fmt.Sprintf("&limit=%d&offset=%d", pagination.Limit, offset)

	url := fmt.Sprintf("%s/rest/v1/categories?%s", s.config.SupabaseURL, query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)

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
			countURL = "&"  filter
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
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)

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
	httpReq.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
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
	httpReq.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
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
		return appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/categories?id=eq.%s", s.config.SupabaseURL, categoryID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
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

	url := fmt.Sprintf("%s/rest/v1/categories?name=eq.%s&select=count", s.config.SupabaseURL, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, appError.New(appError.ErrDatabase, "Database query failed")
	}

	count, err := s.getCount(client, url)
	return count > 0, nil
}

func (s *CategoryService) categoryHasItems(categoryID string) (bool, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/items?category_id=eq.%s&select=count", s.config.SupabaseURL, categoryID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, appError.New(appError.ErrDatabase, "Database query failed")
	}

	count, err := s.getCount(client, url)
	return count > 0, nil
}

func (s *CategoryService) getCount(client *http.Client, url string) (int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)

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

func (s *CategoryService) validateCreateRequest(req *models.CreateCategoryRequest) *appError.AppError {
	if req.Name == "" {
		return appError.New(appError.ErrMissingField, "Category name is required")
	}
	if len(req.Name) < 2 || len(req.Name) > 50 {
		return appError.New(appError.ErrInvalidInput, "Category name must be between 2 and 50 characters")
	}
	if len(req.Description) > 255 {
		return appError.New(appError.ErrInvalidInput, "Description must not exceed 255 characters")
	}
	if len(req.Icon) > 100 {
		return appError.New(appError.ErrInvalidInput, "Icon must not exceed 100 characters")
	}
	return nil
}

func (s *CategoryService) validateUpdateRequest(req *models.UpdateCategoryRequest) *appError.AppError {
	if req.Name != nil {
		if len(*req.Name) < 2 || len(*req.Name) > 50 {
			return appError.New(appError.ErrInvalidInput, "Category name must be between 2 and 50 characters")
		}
	}
	if req.Description != nil && len(*req.Description) > 255 {
		return appError.New(appError.ErrInvalidInput, "Description must not exceed 255 characters")
	}
	if req.Icon != nil && len(*req.Icon) > 100 {
		return appError.New(appError.ErrInvalidInput, "Icon must not exceed 100 characters")
	}
	return nil
}
