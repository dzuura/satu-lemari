package common

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// SuccessResponse represents a standardized success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
	TraceID string      `json:"trace_id,omitempty"`
}

// Meta contains metadata for paginated responses
type Meta struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	Total       int  `json:"total"`
	TotalPages  int  `json:"total_pages"`
	HasNext     bool `json:"has_next"`
	HasPrevious bool `json:"has_previous"`
}

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// GetPaginationParams extracts pagination parameters from request
func GetPaginationParams(r *http.Request) PaginationParams {
	page := GetIntQueryParam(r, "page", 1)
	limit := GetIntQueryParam(r, "limit", 10)

	// Ensure valid ranges
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	return PaginationParams{
		Page:  page,
		Limit: limit,
	}
}

// CalculateMeta calculates metadata for pagination
func CalculateMeta(page, limit, total int) *Meta {
	totalPages := (total + limit - 1) / limit
	hasNext := page < totalPages
	hasPrevious := page > 1

	return &Meta{
		Page:        page,
		Limit:       limit,
		Total:       total,
		TotalPages:  totalPages,
		HasNext:     hasNext,
		HasPrevious: hasPrevious,
	}
}

// WriteSuccessResponse writes a standardized success response
func WriteSuccessResponse(w http.ResponseWriter, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
		TraceID: uuid.New().String(),
	}

	json.NewEncoder(w).Encode(response)
}

// WriteSuccessResponseWithMeta writes a success response with pagination metadata
func WriteSuccessResponseWithMeta(w http.ResponseWriter, data interface{}, meta *Meta, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
		Meta:    meta,
		TraceID: uuid.New().String(),
	}

	json.NewEncoder(w).Encode(response)
}

// WriteCreatedResponse writes a success response for created resources
func WriteCreatedResponse(w http.ResponseWriter, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	response := SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
		TraceID: uuid.New().String(),
	}

	json.NewEncoder(w).Encode(response)
}

// WriteNoContentResponse writes a 204 No Content response
func WriteNoContentResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
