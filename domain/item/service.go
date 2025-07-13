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

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/dzuura/satu-lemari/domain/storage"
)

type ItemService struct {
	config  *config.Config
	storage *storage.SupabaseStorage
}

type SearchFilters struct {
	Search        string
	CategoryID    string
	Type          string
	Status        string
	PartnerID     string
	MinPrice      float64
	MaxPrice      float64
	City          string
	Size          string
	Color         string
	NearLat       float64
	NearLng       float64
	MaxDistance   float64
	OnlyAvailable bool
	SortBy        string
	SortOrder     string
}

func NewItemService(cfg *config.Config) *ItemService {
	return &ItemService{
		config:  cfg,
		storage: storage.NewSupabaseStorage(cfg),
	}
}

func (s *ItemService) RegisterRoutes(r *mux.Router) {
	// Public routes
	r.HandleFunc("/items", s.GetItems).Methods("GET")
	r.HandleFunc("/items/search", s.SearchItems).Methods("GET")
	r.HandleFunc("/items/{item_id}", s.GetItem).Methods("GET")

	// Protected routes (will be handled by middleware)
	// These will be registered in main.go with auth middleware
}

func (s *ItemService) GetItems(w http.ResponseWriter, r *http.Request) {
	filters := s.parseSearchFilters(r)
	pagination := common.GetPaginationParams(r)

	items, total, appErr := s.searchItems(filters, pagination)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
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

	// Increment view count asynchronously
	go s.incrementViewCount(itemID.String())

	common.WriteSuccessResponse(w, item, "Item retrieved successfully")
}

func (s *ItemService) CreateItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Parse form data
	formValues, files, err := common.ParseFormData(r)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Failed to parse form data"),
			common.GenerateTraceID())
		return
	}

	// Validate required fields
	requiredFields := []string{"category_id", "name", "size", "type", "total_quantity", "condition"}
	missingFields := common.ValidateRequiredFormFields(formValues, requiredFields)
	if len(missingFields) > 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Missing required fields: "+strings.Join(missingFields, ", ")),
			common.GenerateTraceID())
		return
	}

	// Parse category_id
	categoryID, err := uuid.Parse(common.GetFormValue(formValues, "category_id"))
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid category_id"),
			common.GenerateTraceID())
		return
	}

	// Parse total_quantity
	totalQuantity, err := common.GetFormInt(formValues, "total_quantity")
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid total_quantity"),
			common.GenerateTraceID())
		return
	}

	// Parse price if provided
	var price *float64
	if priceStr := common.GetFormValue(formValues, "price"); priceStr != "" {
		priceVal, err := common.GetFormFloat(formValues, "price")
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "Invalid price"),
				common.GenerateTraceID())
			return
		}
		price = &priceVal
	}

	// Upload images
	var imageURLs []string
	if uploadedFiles := common.GetFormFiles(files, "images"); len(uploadedFiles) > 0 {
		uploadResults, uploadErr := s.storage.UploadMultipleFiles(uploadedFiles, "items", "images")
		if uploadErr != nil {
			appError.WriteErrorResponse(w, uploadErr, common.GenerateTraceID())
			return
		}

		for _, result := range uploadResults {
			imageURLs = append(imageURLs, result.URL)
		}
	}

	// Create request object
	req := &models.CreateItemRequest{
		CategoryID:    categoryID,
		Name:          common.GetFormValue(formValues, "name"),
		Size:          common.GetFormValue(formValues, "size"),
		Type:          common.GetFormValue(formValues, "type"),
		Price:         price,
		TotalQuantity: totalQuantity,
		Condition:     common.GetFormValue(formValues, "condition"),
		Images:        imageURLs,
	}

	// Set optional fields as pointers
	if description := common.GetFormValueOrDefault(formValues, "description", ""); description != "" {
		req.Description = &description
	}
	if color := common.GetFormValueOrDefault(formValues, "color", ""); color != "" {
		req.Color = &color
	}

	// Validate request
	if err := s.validateCreateRequest(req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Create item
	item, appErr := s.createItem(userID, req)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, item, "Item created successfully")
}

func (s *ItemService) UpdateItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
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
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Forbidden"), common.GenerateTraceID())
		return
	}

	// Parse form data
	formValues, files, err := common.ParseFormData(r)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Failed to parse form data"),
			common.GenerateTraceID())
		return
	}

	// Parse optional fields
	var categoryID *uuid.UUID
	if categoryIDStr := common.GetFormValue(formValues, "category_id"); categoryIDStr != "" {
		if parsed, err := uuid.Parse(categoryIDStr); err == nil {
			categoryID = &parsed
		}
	}

	var name *string
	if nameVal := common.GetFormValue(formValues, "name"); nameVal != "" {
		name = &nameVal
	}

	var description *string
	if descVal := common.GetFormValue(formValues, "description"); descVal != "" {
		description = &descVal
	}

	var size *string
	if sizeVal := common.GetFormValue(formValues, "size"); sizeVal != "" {
		size = &sizeVal
	}

	var color *string
	if colorVal := common.GetFormValue(formValues, "color"); colorVal != "" {
		color = &colorVal
	}

	var price *float64
	if priceStr := common.GetFormValue(formValues, "price"); priceStr != "" {
		if priceVal, err := common.GetFormFloat(formValues, "price"); err == nil {
			price = &priceVal
		}
	}

	var totalQuantity *int
	if qtyStr := common.GetFormValue(formValues, "total_quantity"); qtyStr != "" {
		if qtyVal, err := common.GetFormInt(formValues, "total_quantity"); err == nil {
			totalQuantity = &qtyVal
		}
	}

	var condition *string
	if conditionVal := common.GetFormValue(formValues, "condition"); conditionVal != "" {
		condition = &conditionVal
	}

	var status *string
	if statusVal := common.GetFormValue(formValues, "status"); statusVal != "" {
		status = &statusVal
	}

	// Handle image uploads
	var imageURLs []string
	uploadedFiles := common.GetFormFiles(files, "images")

	// Check if user wants to remove all images (images field is explicitly set to empty)
	removeImages := common.GetFormValue(formValues, "remove_images") == "true"

	if len(uploadedFiles) > 0 {
		// User uploaded new images - delete old ones first
		if len(existing.Images) > 0 {
			log.Printf("Deleting %d old images for item %s", len(existing.Images), itemID.String())
			for _, imageURL := range existing.Images {
				// Extract file path from URL
				// URL format: https://project.supabase.co/storage/v1/object/public/items/images/filename.jpg
				// We need to extract: images/filename.jpg
				if strings.Contains(imageURL, "/storage/v1/object/public/items/") {
					filePath := strings.Split(imageURL, "/storage/v1/object/public/items/")
					if len(filePath) > 1 {
						// Delete file from storage
						if deleteErr := s.storage.DeleteFile("items", filePath[1]); deleteErr != nil {
							log.Printf("Warning: Failed to delete old image %s: %v", imageURL, deleteErr)
							// Continue with upload even if old image deletion fails
						} else {
							log.Printf("Successfully deleted old image: %s", imageURL)
						}
					}
				}
			}
		}

		// Upload new images
		uploadResults, uploadErr := s.storage.UploadMultipleFiles(uploadedFiles, "items", "images")
		if uploadErr != nil {
			appError.WriteErrorResponse(w, uploadErr, common.GenerateTraceID())
			return
		}

		for _, result := range uploadResults {
			imageURLs = append(imageURLs, result.URL)
		}

		log.Printf("Successfully uploaded %d new images for item %s", len(imageURLs), itemID.String())
	} else if removeImages && len(existing.Images) > 0 {
		// User wants to remove all images (no new images uploaded)
		log.Printf("Removing all %d images for item %s", len(existing.Images), itemID.String())
		for _, imageURL := range existing.Images {
			// Extract file path from URL
			if strings.Contains(imageURL, "/storage/v1/object/public/items/") {
				filePath := strings.Split(imageURL, "/storage/v1/object/public/items/")
				if len(filePath) > 1 {
					// Delete file from storage
					if deleteErr := s.storage.DeleteFile("items", filePath[1]); deleteErr != nil {
						log.Printf("Warning: Failed to delete image %s: %v", imageURL, deleteErr)
					} else {
						log.Printf("Successfully deleted image: %s", imageURL)
					}
				}
			}
		}
		// Set empty array to remove all images from database
		imageURLs = []string{}
	}

	// Create request object
	req := &models.UpdateItemRequest{
		CategoryID:    categoryID,
		Name:          name,
		Description:   description,
		Size:          size,
		Color:         color,
		Price:         price,
		TotalQuantity: totalQuantity,
		Condition:     condition,
		Status:        status,
		Images:        imageURLs,
	}

	// Validate request
	if err := s.validateUpdateRequest(req); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Update item
	updatedItem, appErr := s.updateItem(itemID.String(), req)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedItem, "Item updated successfully")
}

func (s *ItemService) UpdateItemStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
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
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Forbidden"), common.GenerateTraceID())
		return
	}

	// Update status
	updateReq := &models.UpdateItemRequest{
		Status: &req.Status,
	}

	updatedItem, appErr := s.updateItem(itemID.String(), updateReq)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedItem, "Item status updated successfully")
}

func (s *ItemService) DeleteItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
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
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Forbidden"), common.GenerateTraceID())
		return
	}

	// Check if item has pending requests
	hasPendingRequests, appErr := s.itemHasPendingRequests(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if hasPendingRequests {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrConflict, "Cannot delete item with pending requests"),
			common.GenerateTraceID())
		return
	}

	// Delete item permanently
	appErr = s.deleteItem(itemID.String())
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, nil, "Item deleted successfully")
}

func (s *ItemService) GetMyItems(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	filters := s.parseSearchFilters(r)
	filters.PartnerID = userID
	pagination := common.GetPaginationParams(r)

	items, total, appErr := s.searchItems(filters, pagination)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, items, meta, "My items retrieved successfully")
}

func (s *ItemService) SearchItems(w http.ResponseWriter, r *http.Request) {
	filters := s.parseSearchFilters(r)
	pagination := common.GetPaginationParams(r)

	items, total, appErr := s.searchItems(filters, pagination)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, items, meta, "Search completed successfully")
}

func (s *ItemService) AnalyzeItemWithAI(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
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
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "Forbidden"), common.GenerateTraceID())
		return
	}

	if !s.config.EnableAIFeatures {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "AI features are disabled"),
			common.GenerateTraceID())
		return
	}

	// Parse form data
	_, files, err := common.ParseFormData(r)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Failed to parse form data"),
			common.GenerateTraceID())
		return
	}

	// Get images to analyze
	var imagesToAnalyze []string

	// If new images are uploaded, use them
	if uploadedFiles := common.GetFormFiles(files, "images"); len(uploadedFiles) > 0 {
		// Upload new images
		uploadResults, uploadErr := s.storage.UploadMultipleFiles(uploadedFiles, "items", "images")
		if uploadErr != nil {
			appError.WriteErrorResponse(w, uploadErr, common.GenerateTraceID())
			return
		}

		for _, result := range uploadResults {
			imagesToAnalyze = append(imagesToAnalyze, result.URL)
		}
	} else {
		// Use existing images from the item
		imagesToAnalyze = existing.Images
	}

	// If no images available, return error
	if len(imagesToAnalyze) == 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "No images available for analysis"),
			common.GenerateTraceID())
		return
	}

	// Analyze item with AI
	go s.analyzeItemWithAI(itemID.String(), imagesToAnalyze)

	common.WriteSuccessResponse(w, map[string]interface{}{
		"message":      "AI analysis started",
		"images_count": len(imagesToAnalyze),
	}, "AI analysis started")
}

// Helper methods

func (s *ItemService) parseSearchFilters(r *http.Request) SearchFilters {
	// Check for both 'search' and 'q' parameters for backward compatibility
	searchParam := common.GetQueryParam(r, "search", "")
	if searchParam == "" {
		searchParam = common.GetQueryParam(r, "q", "")
	}

	filters := SearchFilters{
		Search:        searchParam,
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

	// Build query - include all fields including price
	query := "select=id,name,description,category_id,type,status,price,images,size,color,condition,partner_id,total_quantity,available_quantity,created_at,updated_at"
	sqlFilters := []string{}

	// Basic filters
	if filters.Search != "" {
		// Use OR search on name and description fields like user service
		// Sanitize search parameter
		search := strings.TrimSpace(filters.Search)
		if len(search) > 0 {
			sqlFilters = append(sqlFilters, fmt.Sprintf("or=(name.ilike.*%s*,description.ilike.*%s*)", search, search))
		}
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
	// Note: City filter would need a join with users table - skipping for now
	if filters.Size != "" {
		sqlFilters = append(sqlFilters, fmt.Sprintf("size=eq.%s", filters.Size))
	}
	if filters.Color != "" {
		// Sanitize color parameter
		color := strings.TrimSpace(filters.Color)
		if len(color) > 0 {
			// Use exact match for color instead of ILIKE to avoid issues
			sqlFilters = append(sqlFilters, fmt.Sprintf("color=eq.%s", color))
		}
	}
	if filters.OnlyAvailable {
		sqlFilters = append(sqlFilters, "status=eq.active")
		sqlFilters = append(sqlFilters, "available_quantity=gt.0")
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
		query += "&" + filter
	}

	// Add sorting
	if filters.SortBy != "" && filters.SortOrder != "" {
		query += fmt.Sprintf("&order=%s.%s", filters.SortBy, filters.SortOrder)
	}

	// Add pagination
	offset := (pagination.Page - 1) * pagination.Limit
	query += fmt.Sprintf("&limit=%d&offset=%d", pagination.Limit, offset)

	url := fmt.Sprintf("%s/rest/v1/items?%s", s.config.SupabaseURL, query)
	log.Printf("Search items URL: %s", url)
	log.Printf("SQL filters: %v", sqlFilters)
	log.Printf("Search parameter: '%s'", filters.Search)
	log.Printf("Color parameter: '%s'", filters.Color)
	log.Printf("Final query string: %s", query)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, appError.New(appError.ErrDatabase, "Database connection failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to read response")
	}

	// Log response for debugging
	log.Printf("Supabase response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("Supabase response body: %s", string(body))
	} else {
		// Log first few characters of successful response for debugging
		if len(body) > 200 {
			log.Printf("Supabase response preview: %s...", string(body[:200]))
		} else {
			log.Printf("Supabase response: %s", string(body))
		}
	}

	var items []models.Item
	if err := json.Unmarshal(body, &items); err != nil {
		log.Printf("Failed to parse response body: %s", string(body))
		return nil, 0, appError.New(appError.ErrInternal, "Failed to parse response")
	}

	// Get total count
	countURL := fmt.Sprintf("%s/rest/v1/items?select=count", s.config.SupabaseURL)
	if len(sqlFilters) > 0 {
		for _, filter := range sqlFilters {
			countURL += "&" + filter
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

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

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
		"id":                 uuid.New().String(),
		"name":               req.Name,
		"description":        req.Description,
		"category_id":        req.CategoryID,
		"type":               req.Type,
		"status":             "active",
		"price":              req.Price,
		"images":             req.Images,
		"size":               req.Size,
		"color":              req.Color,
		"condition":          req.Condition,
		"partner_id":         partnerID,
		"total_quantity":     req.TotalQuantity,
		"available_quantity": req.TotalQuantity, // Initially same as total
		"created_at":         time.Now().Format(time.RFC3339),
		"updated_at":         time.Now().Format(time.RFC3339),
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

	httpReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
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
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = *req.Description
	}
	if req.CategoryID != nil {
		updateData["category_id"] = *req.CategoryID
	}
	if req.Status != nil {
		updateData["status"] = *req.Status
	}
	if req.Price != nil {
		updateData["price"] = *req.Price
	}
	if req.Images != nil {
		if len(req.Images) > 0 {
			updateData["images"] = req.Images
		}
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

	httpReq.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	httpReq.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
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
	// First, get the item to extract image URLs
	item, appErr := s.getItemByID(itemID)
	if appErr != nil {
		return appErr
	}

	// Delete images from storage if they exist
	if len(item.Images) > 0 {
		for _, imageURL := range item.Images {
			// Extract file path from URL
			// URL format: https://project.supabase.co/storage/v1/object/public/items/images/filename.jpg
			// We need to extract: images/filename.jpg
			if strings.Contains(imageURL, "/storage/v1/object/public/items/") {
				filePath := strings.Split(imageURL, "/storage/v1/object/public/items/")
				if len(filePath) > 1 {
					// Delete file from storage
					if deleteErr := s.storage.DeleteFile("items", filePath[1]); deleteErr != nil {
						log.Printf("Warning: Failed to delete image %s: %v", imageURL, deleteErr)
						// Continue with deletion even if image deletion fails
					} else {
						log.Printf("Successfully deleted image: %s", imageURL)
					}
				}
			}
		}
	}

	// Delete item from database
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to delete item")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to delete item (status %d): %s", resp.StatusCode, string(bodyBytes))
		return appError.New(appError.ErrDatabase, "Failed to delete item in database")
	}

	log.Printf("Successfully deleted item %s and all associated images", itemID)

	return nil
}

func (s *ItemService) incrementViewCount(_ string) {
	// TODO: Add view_count column to database schema if needed
	// For now, this function is disabled as view_count column doesn't exist
	log.Printf("View count increment disabled - column not in schema")
}

func (s *ItemService) itemHasPendingRequests(itemID string) (bool, *appError.AppError) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/requests?item_id=eq.%s&status=eq.pending&select=count", s.config.SupabaseURL, itemID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, appError.New(appError.ErrInternal, "Failed to create request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

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

func (s *ItemService) analyzeItemWithAI(_ string, _ []string) {
	// TODO: Add ai_analysis column to database schema if needed
	// For now, this function is disabled as ai_analysis column doesn't exist
	log.Printf("AI analysis disabled - column not in schema")
}

func (s *ItemService) getCount(client *http.Client, url string) (int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

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

func (s *ItemService) validateCreateRequest(req *models.CreateItemRequest) *appError.AppError {
	if req.Name == "" {
		return appError.New(appError.ErrMissingField, "Name is required")
	}
	if len(req.Name) < 3 || len(req.Name) > 100 {
		return appError.New(appError.ErrInvalidInput, "Name must be between 3 and 100 characters")
	}
	if req.CategoryID == uuid.Nil {
		return appError.New(appError.ErrMissingField, "Category ID is required")
	}
	if req.Type == "" {
		return appError.New(appError.ErrMissingField, "Type is required")
	}
	if !common.Contains([]string{"donation", "rental"}, req.Type) {
		return appError.New(appError.ErrInvalidInput, "Type must be either 'donation' or 'rental'")
	}
	if req.Type == "rental" && (req.Price == nil || *req.Price <= 0) {
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
	if req.Name != nil {
		if len(*req.Name) < 3 || len(*req.Name) > 100 {
			return appError.New(appError.ErrInvalidInput, "Name must be between 3 and 100 characters")
		}
	}
	if req.Status != nil {
		if !common.Contains([]string{"active", "inactive", "rented", "donated"}, *req.Status) {
			return appError.New(appError.ErrInvalidInput, "Invalid status")
		}
	}
	if req.Images != nil {
		if len(req.Images) == 0 {
			return appError.New(appError.ErrInvalidInput, "At least one image is required")
		}
		if len(req.Images) > 5 {
			return appError.New(appError.ErrInvalidInput, "Maximum 5 images allowed")
		}
	}
	return nil
}
