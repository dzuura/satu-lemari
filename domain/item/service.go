
package item

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/google/uuid"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type ItemService struct {
	config *config.Config
}

type SearchFilters struct {
	Search         string
	CategoryID     string
	Type           string
	Status         string
	PartnerID      string
	MinPrice       float64
	MaxPrice       float64
	City           string
	Size           string
	Color          string
	NearLat        float64
	NearLng        float64
	MaxDistance    float64
	OnlyAvailable  bool
	SortBy         string
	SortOrder      string
}

func NewItemService(cfg *config.Config) *ItemService {
	return &ItemService{
		config: cfg,
	}
}

func (s *ItemService) RegisterRoutes(r *mux.Router) {
	// Public routes
	r.HandleFunc("/items", s.GetItems).Methods("GET")
	r.HandleFunc("/items/{item_id}", s.GetItem).Methods("GET")
	r.HandleFunc("/items/search", s.SearchItems).Methods("GET")
	
	// Partner routes - require authentication
	r.HandleFunc("/items", s.CreateItem).Methods("POST")
	r.HandleFunc("/items/{item_id}", s.UpdateItem).Methods("PUT")
	r.HandleFunc("/items/{item_id}/status", s.UpdateItemStatus).Methods("PATCH")
	r.HandleFunc("/items/{item_id}", s.DeleteItem).Methods("DELETE")
	r.HandleFunc("/items/{item_id}/ai-analyze", s.AnalyzeItemWithAI).Methods("POST")
	
	// My items
	r.HandleFunc("/items/mine", s.GetMyItems).Methods("GET")
}

func (s *ItemService) GetItems(w http.ResponseWriter, r *http.Request) {
	filters := s.parseSearchFilters(r)
	pagination := common.GetPaginationParams(r)

	items, total, err := s.searchItems(filters, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, items, meta, "Items retrieved successfully")
}

func (s *ItemService) GetItem(w http.ResponseWriter, r *http.Request) {
	itemIDStr := common.GetPathParam(r, "item_id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item ID"),
			common.GenerateTraceID())
		return
	}

	item, appErr := s.getItemByID(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Increment view count
	go s.incrementViewCount(itemID.String())

	common.WriteSuccessResponse(w, item, "Item retrieved successfully")
}

func (s *ItemService) CreateItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	role, ok := common.GetUserRoleFromContext(r)
	if !ok || role != "partner" {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	var req models.CreateItemRequest
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

	// Create item
	item, err := s.createItem(userID, &req)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// If AI analysis is enabled and we have images, analyze them
	if s.config.OpenAIAPIKey != "" && len(req.Images) > 0 {
		go s.analyzeItemWithAI(item.ID, req.Images)
	}

	common.WriteSuccessResponse(w, item, "Item created successfully")
}

func (s *ItemService) UpdateItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	itemIDStr := common.GetPathParam(r, "item_id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item ID"),
			common.GenerateTraceID())
		return
	}

	// Check if user owns this item
	existing, appErr := s.getItemByID(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if existing.PartnerID != userID {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	var req models.UpdateItemRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate request
	if err := s.validateUpdateRequest(&req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Update item
	updatedItem, err := s.updateItem(itemID.String(), &req)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedItem, "Item updated successfully")
}

func (s *ItemService) UpdateItemStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	itemIDStr := common.GetPathParam(r, "item_id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item ID"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=active inactive"`
	}
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Check if user owns this item
	existing, appErr := s.getItemByID(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if existing.PartnerID != userID {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	// Update status
	updateReq := &models.UpdateItemRequest{
		Status: &req.Status,
	}

	updatedItem, err := s.updateItem(itemID.String(), updateReq)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedItem, "Item status updated successfully")
}

func (s *ItemService) DeleteItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	itemIDStr := common.GetPathParam(r, "item_id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item ID"),
			common.GenerateTraceID())
		return
	}

	// Check if user owns this item
	existing, appErr := s.getItemByID(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if existing.PartnerID != userID {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	// Check if item has pending requests
	hasPendingRequests, err := s.itemHasPendingRequests(itemID.String())
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	if hasPendingRequests {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrItemInUse, "Cannot delete item with pending requests"),
			common.GenerateTraceID())
		return
	}

	// Soft delete item
	err = s.deleteItem(itemID.String())
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, nil, "Item deleted successfully")
}

func (s *ItemService) GetMyItems(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	filters := s.parseSearchFilters(r)
	filters.PartnerID = userID
	pagination := common.GetPaginationParams(r)

	items, total, err := s.searchItems(filters, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, items, meta, "My items retrieved successfully")
}

func (s *ItemService) SearchItems(w http.ResponseWriter, r *http.Request) {
	filters := s.parseSearchFilters(r)
	pagination := common.GetPaginationParams(r)

	items, total, err := s.searchItems(filters, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, items, meta, "Search completed successfully")
}

func (s *ItemService) AnalyzeItemWithAI(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.ErrUnauthorized,
			common.GenerateTraceID())
		return
	}

	itemIDStr := common.GetPathParam(r, "item_id")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item ID"),
			common.GenerateTraceID())
		return
	}

	// Check if user owns this item
	existing, appErr := s.getItemByID(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if existing.PartnerID != userID {
		appError.WriteErrorResponse(w,
			appError.ErrForbidden,
			common.GenerateTraceID())
		return
	}

	if s.config.OpenAIAPIKey == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrServiceUnavailable, "AI analysis service is not configured"),
			common.GenerateTraceID())
		return
	}

	// Start AI analysis in background
	go s.analyzeItemWithAI(itemID.String(), existing.Images)

	common.WriteSuccessResponse(w, map[string]interface{}{
		"message": "AI analysis started",
		"item_id": itemID.String(),
	}, "AI analysis initiated successfully")
}

// Helper methods

func (s *ItemService) parseSearchFilters(r *http.Request) SearchFilters {
	filters := SearchFilters{
		Search:        common.GetQueryParam(r, "search", ""),
		CategoryID:    common.GetQueryParam(r, "category_id", ""),
		Type:          common.GetQueryParam(r, "type", ""),
		Status:        common.GetQueryParam(r, "status", "active"),
		PartnerID:     common.GetQueryParam(r, "partner_id", ""),
		City:          common.GetQueryParam(r, "city", ""),
		Size:          common.GetQueryParam(r, "size", ""),
		Color:         common.GetQueryParam(r, "color", ""),
		OnlyAvailable: common.GetQueryParam(r, "available", "") == "true",
		SortBy:        common.GetQueryParam(r, "sort_by", "created_at"),
		SortOrder:     common.GetQueryParam(r, "sort_order", "desc"),
	}

	// Parse price range
	if minPriceStr := common.GetQueryParam(r, "min_price", ""); minPriceStr != "" {
		if minPrice, err := strconv.ParseFloat(minPriceStr, 64); err == nil {
			filters.MinPrice = minPrice
		}
	}
	if maxPriceStr := common.GetQueryParam(r, "max_price", ""); maxPriceStr != "" {
		if maxPrice, err := strconv.ParseFloat(maxPriceStr, 64); err == nil {
			filters.MaxPrice = maxPrice
		}
	}

	// Parse location
	if latStr := common.GetQueryParam(r, "lat", ""); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			filters.NearLat = lat
		}
	}
	if lngStr := common.GetQueryParam(r, "lng", ""); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			filters.NearLng = lng
		}
	}
	if distStr := common.GetQueryParam(r, "max_distance", "10"); distStr != "" {
		if dist, err := strconv.ParseFloat(distStr, 64); err == nil {
			filters.MaxDistance = dist
		}
	}

	return filters
}

func (s *ItemService) searchItems(filters SearchFilters, pagination common.PaginationParams) ([]models.Item, int, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Build query
	query := "select=id,title,description,category_id,type,status,price,images,size,color,condition,partner_id,is_available,view_count,ai_analysis,created_at,updated_at"
	sqlFilters := []string{}

	// Basic filters
	if filters.Search != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("or=(title.ilike.%%%s%%,description.ilike.%%%s%%)", filters.Search, filters.Search))
	}
	if filters.CategoryID != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("category_id=eq.%s", filters.CategoryID))
	}
	if filters.Type != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("type=eq.%s", filters.Type))
	}
	if filters.Status != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("status=eq.%s", filters.Status))
	}
	if filters.PartnerID != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("partner_id=eq.%s", filters.PartnerID))
	}
	if filters.City != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("partner_id=eq.%s", filters.City)) // This would need a join with users table
	}
	if filters.Size != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("size=eq.%s", filters.Size))
	}
	if filters.Color != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("color.ilike.%%%s%%", filters.Color))
	}
	if filters.OnlyAvailable {
		sqlFilters = append(sqlFilters, "is_available=eq.true")
	}

	// Price range
	if filters.MinPrice > 0 {
		sqlFilters = append(sqlFilters, fmt.Sprintf("price=gte.%f", filters.MinPrice))
	}
	if filters.MaxPrice > 0 {
		sqlFilters = append(sqlFilters, fmt.Sprintf("price=lte.%f", filters.MaxPrice))
	}

	// Add filters to query
	for _, filter := range sqlFilters {
		query = "&"  filter
	}

	// Add sorting
	if filters.SortBy != "" && filters.SortOrder != "" {
		query = fmt.Sprintf("&order=%s.%s", filters.SortBy, filters.SortOrder)
	}

	// Add pagination
	offset := (pagination.Page - 1) * pagination.Limit
	query = fmt.Sprintf("&limit=%d&offset=%d", pagination.Limit, offset)

	url := fmt.Sprintf("%s/rest/v1/items?%s", s.config.SupabaseURL, query)
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

	var items []models.Item
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	// Get total count
	countURL := fmt.Sprintf("%s/rest/v1/items?select=count", s.config.SupabaseURL)
	if len(sqlFilters) > 0 {
		for _, filter := range sqlFilters {
			countURL = "&"  filter
		}
	}

	total, _ := s.getCount(client, countURL)

	return items, total, nil
}

func (s *ItemService) getItemByID(itemID string) (*models.Item, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=*", s.config.SupabaseURL, itemID)
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

	var items []models.Item
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	if len(items) == 0 {
		return nil, appError.ErrItemNotFound
	}

	return &items[0], nil
}

func (s *ItemService) createItem(partnerID string, req *models.CreateItemRequest) (*models.Item, *appError.AppError) {
	newItem := map[string]interface{}{
		"id":           uuid.New().String(),
		"title":        req.Title,
		"description":  req.Description,
		"category_id":  req.CategoryID,
		"type":         req.Type,
		"status":       "active",
		"price":        req.Price,
		"images":       req.Images,
		"size":         req.Size,
		"color":        req.Color,
		"condition":    req.Condition,
		"partner_id":   partnerID,
		"is_available": true,
		"view_count":   0,
		"created_at":   time.Now().Format(time.RFC3339),
		"updated_at":   time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(newItem)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal item data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items", s.config.SupabaseURL)
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
		return nil, appError.New(appError.ErrDatabase, "Failed to create item")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to create item: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Failed to create item in database")
	}

	// Parse created item
	var createdItems []models.Item
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &createdItems); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse created item")
	}

	if len(createdItems) == 0 {
		return nil, appError.New(appError.ErrInternal, "No item returned from creation")
	}

	return &createdItems[0], nil
}

func (s *ItemService) updateItem(itemID string, req *models.UpdateItemRequest) (*models.Item, *appError.AppError) {
	updateData := map[string]interface{}{
		"updated_at": time.Now().Format(time.RFC3339),
	}

	// Only include non-nil fields
	if req.Title != nil {
		updateData["title"] = *req.Title
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if req.CategoryID != nil {
		updateData["category_id"] = *req.CategoryID
	}
	if req.Type != nil {
		updateData["type"] = *req.Type
	}
	if req.Status != nil {
		updateData["status"] = *req.Status
	}
	if req.Price != nil {
		updateData["price"] = *req.Price
	}
	if req.Images != nil {
		updateData["images"] = *req.Images
	}
	if req.Size != nil {
		updateData["size"] = *req.Size
	}
	if req.Color != nil {
		updateData["color"] = *req.Color
	}
	if req.Condition != nil {
		updateData["condition"] = *req.Condition
	}
	if req.IsAvailable != nil {
		updateData["is_available"] = *req.IsAvailable
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
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
		return nil, appError.New(appError.ErrDatabase, "Failed to update item")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to update item: %s", string(bodyBytes))
		return nil, appError.New(appError.ErrDatabase, "Failed to update item in database")
	}

	// Parse updated item
	var updatedItems []models.Item
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response")
	}

	if err := json.Unmarshal(bodyBytes, &updatedItems); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse updated item")
	}

	if len(updatedItems) == 0 {
		return nil, appError.New(appError.ErrInternal, "No item returned from update")
	}

	return &updatedItems[0], nil
}

func (s *ItemService) deleteItem(itemID string) *appError.AppError {
	updateData := map[string]interface{}{
		"status":     "deleted",
		"updated_at": time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to delete item")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to delete item: %s", string(bodyBytes))
		return appError.New(appError.ErrDatabase, "Failed to delete item in database")
	}

	return nil
}

func (s *ItemService) incrementViewCount(itemID string) {
	updateData := map[string]interface{}{
		"view_count": "view_count  1",
		"updated_at": time.Now().Format(time.RFC3339),
	}

	jsonData, _ := json.Marshal(updateData)
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client.Do(req)
}

func (s *ItemService) itemHasPendingRequests(itemID string) (bool, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/requests?item_id=eq.%s&status=eq.pending&select=count", s.config.SupabaseURL, itemID)
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

	count, _ := s.getCount(client, url)
	return count > 0, nil
}

func (s *ItemService) analyzeItemWithAI(itemID string, images []string) {
	if s.config.OpenAIAPIKey == "" || len(images) == 0 {
		return
	}

	// This is a simplified AI analysis - in production you would call OpenAI Vision API
	analysisData := map[string]interface{}{
		"ai_analysis": map[string]interface{}{
			"analyzed_at": time.Now().Format(time.RFC3339),
			"confidence":  0.85,
			"tags":        []string{"casual", "cotton", "good_condition"},
			"description": "AI-generated description based on image analysis",
		},
		"updated_at": time.Now().Format(time.RFC3339),
	}

	jsonData, _ := json.Marshal(analysisData)
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))

	req.Header.Set("apikey", s.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "s.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}

	log.Printf("AI analysis completed for item %s", itemID)
}

func (s *ItemService) getCount(client *http.Client, url string) (int, error) {
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

func (s *ItemService) validateCreateRequest(req *models.CreateItemRequest) *appError.AppError {
	if req.Title == "" {
		return appError.New(appError.ErrMissingField, "Title is required")
	}
	if len(req.Title) < 3 || len(req.Title) > 100 {
		return appError.New(appError.ErrInvalidInput, "Title must be between 3 and 100 characters")
	}
	if req.CategoryID == "" {
		return appError.New(appError.ErrMissingField, "Category ID is required")
	}
	if req.Type == "" {
		return appError.New(appError.ErrMissingField, "Type is required")
	}
	if !common.Contains([]string{"donation", "rental"}, req.Type) {
		return appError.New(appError.ErrInvalidInput, "Type must be either 'donation' or 'rental'")
	}
	if req.Type == "rental" && req.Price <= 0 {
		return appError.New(appError.ErrInvalidInput, "Price is required for rental items")
	}
	if len(req.Images) == 0 {
		return appError.New(appError.ErrMissingField, "At least one image is required")
	}
	if len(req.Images) > 5 {
		return appError.New(appError.ErrInvalidInput, "Maximum 5 images allowed")
	}
	return nil
}

func (s *ItemService) validateUpdateRequest(req *models.UpdateItemRequest) *appError.AppError {
	if req.Title != nil {
		if len(*req.Title) < 3 || len(*req.Title) > 100 {
			return appError.New(appError.ErrInvalidInput, "Title must be between 3 and 100 characters")
		}
	}
	if req.Type != nil {
		if !common.Contains([]string{"donation", "rental"}, *req.Type) {
			return appError.New(appError.ErrInvalidInput, "Type must be either 'donation' or 'rental'")
		}
	}
	if req.Status != nil {
		if !common.Contains([]string{"active", "inactive", "rented", "donated"}, *req.Status) {
			return appError.New(appError.ErrInvalidInput, "Invalid status")
		}
	}
	if req.Images != nil {
		if len(*req.Images) == 0 {
			return appError.New(appError.ErrInvalidInput, "At least one image is required")
		}
		if len(*req.Images) > 5 {
			return appError.New(appError.ErrInvalidInput, "Maximum 5 images allowed")
		}
	}
	return nil
}