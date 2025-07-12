package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// AIServiceHandler handles AI-related HTTP requests
type AIServiceHandler struct {
	config *config.Config
	ai     *AIServiceManager
}

// NewAIServiceHandler creates a new AI service handler
func NewAIServiceHandler(cfg *config.Config, aiManager *AIServiceManager) *AIServiceHandler {
	return &AIServiceHandler{
		config: cfg,
		ai:     aiManager,
	}
}

// RegisterRoutes registers AI service routes
func (h *AIServiceHandler) RegisterRoutes(r *mux.Router) {
	// AI Smart Listing endpoints
	r.HandleFunc("/ai/smart-listing", h.SmartListing).Methods("POST")
	r.HandleFunc("/ai/smart-listing/batch", h.BatchSmartListing).Methods("POST")

	// AI Intent Matching endpoints
	r.HandleFunc("/ai/intent", h.ParseIntent).Methods("POST")
	r.HandleFunc("/ai/suggestions", h.GetSuggestions).Methods("GET")

	// AI Status and info
	r.HandleFunc("/ai/status", h.GetAIStatus).Methods("GET")

	// Legacy AI endpoints (for backward compatibility)
	r.HandleFunc("/ai/analyze", h.AnalyzeItem).Methods("POST")
	r.HandleFunc("/ai/recommendations", h.GetRecommendations).Methods("POST")
}

// SmartListingRequest represents request for smart listing
type SmartListingRequest struct {
	Images      []string          `json:"images" validate:"required,min=1"` // Base64 encoded images
	BasicInfo   map[string]string `json:"basic_info,omitempty"`
	Description string            `json:"description,omitempty"`
}

// SmartListingResponse represents response from smart listing
type SmartListingResponse struct {
	Success bool                `json:"success"`
	Data    *SmartListingResult `json:"data,omitempty"`
	Error   string              `json:"error,omitempty"`
}

// IntentRequest represents request for intent parsing
type IntentRequest struct {
	Query string `json:"query" validate:"required"`
}

// IntentResponse represents response from intent parsing
type IntentResponse struct {
	Success bool          `json:"success"`
	Data    *IntentResult `json:"data,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// SmartListing handles smart listing requests
func (h *AIServiceHandler) SmartListing(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated (optional for AI endpoints)
	userID, _ := common.GetUserIDFromContext(r)

	var req SmartListingRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate request
	if len(req.Images) == 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrMissingField, "At least one image is required"),
			common.GenerateTraceID())
		return
	}

	// Convert base64 images to bytes
	var imageBytes [][]byte
	for i, base64Image := range req.Images {
		// Remove data URL prefix if present
		if strings.HasPrefix(base64Image, "data:image/") {
			parts := strings.Split(base64Image, ",")
			if len(parts) != 2 {
				appError.WriteErrorResponse(w,
					appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid image format at index %d", i)),
					common.GenerateTraceID())
				return
			}
			base64Image = parts[1]
		}

		imageData, err := base64.StdEncoding.DecodeString(base64Image)
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid base64 image at index %d", i)),
				common.GenerateTraceID())
			return
		}

		imageBytes = append(imageBytes, imageData)
	}

	// Prepare basic info
	basicInfo := make(map[string]string)
	if req.BasicInfo != nil {
		basicInfo = req.BasicInfo
	}
	if req.Description != "" {
		basicInfo["description"] = req.Description
	}

	// Process with AI
	ctx := context.Background()
	result, err := h.ai.AutoListItems(ctx, imageBytes, basicInfo)
	if err != nil {
		log.Printf("Smart listing error for user %s: %v", userID, err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to analyze images"),
			common.GenerateTraceID())
		return
	}

	response := SmartListingResponse{
		Success: true,
		Data:    result,
	}

	common.WriteSuccessResponse(w, response, "Smart listing analysis completed")
}

// BatchSmartListing handles batch smart listing requests
func (h *AIServiceHandler) BatchSmartListing(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated
	_, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Parse multipart form for batch processing
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Failed to parse form data"),
			common.GenerateTraceID())
		return
	}

	// Get uploaded files
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrMissingField, "No images uploaded"),
			common.GenerateTraceID())
		return
	}

	// Process each image
	var results []*SmartListingResult
	for _, file := range files {
		// Open file
		src, err := file.Open()
		if err != nil {
			log.Printf("Failed to open file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		// Read file data
		imageData, err := io.ReadAll(src)
		if err != nil {
			log.Printf("Failed to read file %s: %v", file.Filename, err)
			continue
		}

		// Process single image
		basicInfo := map[string]string{
			"filename": file.Filename,
		}

		ctx := context.Background()
		result, err := h.ai.AutoListItems(ctx, [][]byte{imageData}, basicInfo)
		if err != nil {
			log.Printf("Failed to analyze file %s: %v", file.Filename, err)
			continue
		}

		results = append(results, result)
	}

	if len(results) == 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to analyze any images"),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, results, fmt.Sprintf("Successfully analyzed %d images", len(results)))
}

// ParseIntent handles intent parsing requests
func (h *AIServiceHandler) ParseIntent(w http.ResponseWriter, r *http.Request) {
	var req IntentRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate request
	if req.Query == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrMissingField, "Query is required"),
			common.GenerateTraceID())
		return
	}

	// Process with AI
	ctx := context.Background()
	result, err := h.ai.ParseIntent(ctx, req.Query)
	if err != nil {
		log.Printf("Intent parsing error: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to parse intent"),
			common.GenerateTraceID())
		return
	}

	response := IntentResponse{
		Success: true,
		Data:    result,
	}

	common.WriteSuccessResponse(w, response, "Intent parsed successfully")
}

// GetSuggestions handles search suggestions requests
func (h *AIServiceHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	query := common.GetQueryParam(r, "q", "")
	if query == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrMissingField, "Query parameter 'q' is required"),
			common.GenerateTraceID())
		return
	}

	// Process with AI
	ctx := context.Background()
	suggestions, err := h.ai.GetSearchSuggestions(ctx, query)
	if err != nil {
		log.Printf("Suggestions error: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to generate suggestions"),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"query":       query,
		"suggestions": suggestions,
	}

	common.WriteSuccessResponse(w, response, "Suggestions generated successfully")
}

// GetAIStatus returns the status of AI services
func (h *AIServiceHandler) GetAIStatus(w http.ResponseWriter, r *http.Request) {
	status := h.ai.GetAIServiceStatus()
	common.WriteSuccessResponse(w, status, "AI service status retrieved")
}

// AnalyzeItem handles legacy item analysis requests
func (h *AIServiceHandler) AnalyzeItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string `json:"description" validate:"required"`
		ImageData   string `json:"image_data,omitempty"` // Base64 encoded
	}

	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Convert image data if provided
	var imageBytes []byte
	if req.ImageData != "" {
		var err error
		imageBytes, err = base64.StdEncoding.DecodeString(req.ImageData)
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "Invalid image data"),
				common.GenerateTraceID())
			return
		}
	}

	// Process with AI
	ctx := context.Background()
	result, err := h.ai.AnalyzeItem(ctx, req.Description, imageBytes)
	if err != nil {
		log.Printf("Item analysis error: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to analyze item"),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, result, "Item analysis completed")
}

// GetRecommendations handles legacy recommendation requests
func (h *AIServiceHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserProfile    map[string]interface{}   `json:"user_profile" validate:"required"`
		AvailableItems []map[string]interface{} `json:"available_items" validate:"required"`
	}

	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Process with AI
	ctx := context.Background()
	recommendations, err := h.ai.GenerateRecommendations(ctx, req.UserProfile, req.AvailableItems)
	if err != nil {
		log.Printf("Recommendations error: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to generate recommendations"),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, recommendations, "Recommendations generated successfully")
}
