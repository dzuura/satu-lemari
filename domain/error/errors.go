package error

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorType represents different types of errors
type ErrorType string

const (
	// Authentication and Authorization errors
	ErrUnauthorized     ErrorType = "UNAUTHORIZED"
	ErrForbidden        ErrorType = "FORBIDDEN"
	ErrTokenInvalid     ErrorType = "TOKEN_INVALID"
	ErrTokenExpired     ErrorType = "TOKEN_EXPIRED"
	
	// Validation errors
	ErrValidation       ErrorType = "VALIDATION_ERROR"
	ErrInvalidInput     ErrorType = "INVALID_INPUT"
	ErrMissingField     ErrorType = "MISSING_FIELD"
	ErrDuplicateEntry   ErrorType = "DUPLICATE_ENTRY"
	
	// Resource errors
	ErrNotFound         ErrorType = "NOT_FOUND"
	ErrAlreadyExists    ErrorType = "ALREADY_EXISTS"
	ErrConflict         ErrorType = "CONFLICT"
	
	// Business logic errors
	ErrQuotaExceeded    ErrorType = "QUOTA_EXCEEDED"
	ErrInsufficientStock ErrorType = "INSUFFICIENT_STOCK"
	ErrItemNotAvailable ErrorType = "ITEM_NOT_AVAILABLE"
	ErrInvalidOperation ErrorType = "INVALID_OPERATION"
	
	// System errors
	ErrInternal         ErrorType = "INTERNAL_ERROR"
	ErrDatabase         ErrorType = "DATABASE_ERROR"
	ErrExternal         ErrorType = "EXTERNAL_SERVICE_ERROR"
	ErrRateLimited      ErrorType = "RATE_LIMITED"
)

// AppError represents a structured application error
type AppError struct {
	Type    ErrorType   `json:"type"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
	Code    string      `json:"code,omitempty"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// ErrorResponse represents the standardized error response format
type ErrorResponse struct {
	Error   *AppError `json:"error"`
	Success bool      `json:"success"`
	TraceID string    `json:"trace_id,omitempty"`
}

// New creates a new AppError
func New(errType ErrorType, message string) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
	}
}

// NewWithDetails creates a new AppError with additional details
func NewWithDetails(errType ErrorType, message string, details interface{}) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Details: details,
	}
}

// NewWithCode creates a new AppError with error code
func NewWithCode(errType ErrorType, message string, code string) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Code:    code,
	}
}

// GetHTTPStatusCode returns the appropriate HTTP status code for an error type
func GetHTTPStatusCode(errType ErrorType) int {
	switch errType {
	case ErrUnauthorized, ErrTokenInvalid, ErrTokenExpired:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	case ErrValidation, ErrInvalidInput, ErrMissingField:
		return http.StatusBadRequest
	case ErrNotFound:
		return http.StatusNotFound
	case ErrAlreadyExists, ErrDuplicateEntry:
		return http.StatusConflict
	case ErrConflict, ErrQuotaExceeded, ErrInsufficientStock, ErrItemNotAvailable, ErrInvalidOperation:
		return http.StatusConflict
	case ErrRateLimited:
		return http.StatusTooManyRequests
	case ErrInternal, ErrDatabase, ErrExternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// WriteErrorResponse writes a standardized error response
func WriteErrorResponse(w http.ResponseWriter, err *AppError, traceID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(GetHTTPStatusCode(err.Type))
	
	response := ErrorResponse{
		Error:   err,
		Success: false,
		TraceID: traceID,
	}
	
	json.NewEncoder(w).Encode(response)
}

// Predefined common errors
var (
	ErrUserNotFound = New(ErrNotFound, "User not found")
	ErrItemNotFound = New(ErrNotFound, "Item not found")
	ErrRequestNotFound = New(ErrNotFound, "Request not found")
	ErrCategoryNotFound = New(ErrNotFound, "Category not found")
	
	ErrInvalidCredentials = New(ErrUnauthorized, "Invalid credentials")
	ErrAccessDenied = New(ErrForbidden, "Access denied")
	
	ErrEmailAlreadyExists = New(ErrDuplicateEntry, "Email already exists")
	ErrUsernameAlreadyExists = New(ErrDuplicateEntry, "Username already exists")
	
	ErrWeeklyQuotaExceeded = New(ErrQuotaExceeded, "Weekly donation quota exceeded")
	ErrItemOutOfStock = New(ErrInsufficientStock, "Item is out of stock")
	
	ErrInternalServer = New(ErrInternal, "Internal server error")
	ErrDatabaseConnection = New(ErrDatabase, "Database connection error")
)

// ValidationError represents field validation errors
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

// NewValidationError creates a validation error with multiple field errors
func NewValidationError(fields []ValidationError) *AppError {
	return NewWithDetails(ErrValidation, "Validation failed", map[string]interface{}{
		"fields": fields,
	})
}