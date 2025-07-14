package common

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Context key types to avoid collisions
type contextKey string

const (
	userIDKey        contextKey = "user_id"
	userRoleKey      contextKey = "user_role"
	userEmailKey     contextKey = "user_email"
	usernameKey      contextKey = "username"
	requestIDKey     contextKey = "request_id"
	tokenClaimsKey   contextKey = "token_claims"
	authenticatedKey contextKey = "authenticated"
)

// Context key getters
func UserIDKey() contextKey        { return userIDKey }
func UserRoleKey() contextKey      { return userRoleKey }
func UserEmailKey() contextKey     { return userEmailKey }
func UsernameKey() contextKey      { return usernameKey }
func RequestIDKey() contextKey     { return requestIDKey }
func TokenClaimsKey() contextKey   { return tokenClaimsKey }
func AuthenticatedKey() contextKey { return authenticatedKey }

// GetQueryParam returns query parameter value or default
func GetQueryParam(r *http.Request, key, defaultValue string) string {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetIntQueryParam returns query parameter as integer or default
func GetIntQueryParam(r *http.Request, key string, defaultValue int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return intValue
}

// GetBoolQueryParam returns query parameter as boolean or default
func GetBoolQueryParam(r *http.Request, key string, defaultValue bool) bool {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return boolValue
}

// GetPathParam returns path parameter from mux router
func GetPathParam(r *http.Request, key string) string {
	vars := mux.Vars(r)
	return vars[key]
}

// GetUUIDParam returns path parameter as UUID
func GetUUIDParam(r *http.Request, key string) (uuid.UUID, error) {
	param := GetPathParam(r, key)
	return uuid.Parse(param)
}

// ParseJSONBody parses JSON request body into struct
func ParseJSONBody(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// IsValidEmail checks if email format is valid
func IsValidEmail(email string) bool {
	// Basic email validation
	if len(email) < 3 || len(email) > 254 {
		return false
	}

	atIndex := strings.LastIndex(email, "@")
	if atIndex < 1 || atIndex >= len(email)-1 {
		return false
	}

	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]

	// Check local part
	if len(localPart) < 1 || len(localPart) > 64 {
		return false
	}

	// Check domain part
	if len(domainPart) < 1 || len(domainPart) > 253 {
		return false
	}

	// Check for at least one dot in domain
	if !strings.Contains(domainPart, ".") {
		return false
	}

	return true
}

// IsValidUsername checks if username format is valid
func IsValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 30 {
		return false
	}

	// Username can contain alphanumeric characters, underscores, and spaces
	for _, char := range username {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' && char != ' ' {
			return false
		}
	}

	// Should not start or end with underscore or space
	username = strings.TrimSpace(username)
	if strings.HasPrefix(username, "_") || strings.HasSuffix(username, "_") {
		return false
	}

	return true
}

// IsValidPhoneNumber checks if phone number format is valid (Indonesian format)
func IsValidPhoneNumber(phone string) bool {
	// Remove spaces and dashes
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	// Check if it starts with 62 or 08
	if strings.HasPrefix(phone, "62") {
		phone = phone[3:]
	} else if strings.HasPrefix(phone, "08") {
		phone = phone[2:]
	} else {
		return false
	}

	// Check remaining digits
	if len(phone) < 8 || len(phone) > 13 {
		return false
	}

	// Should contain only digits
	for _, char := range phone {
		if !unicode.IsDigit(char) {
			return false
		}
	}

	return true
}

// GenerateTraceID generates a unique trace ID for request tracking
func GenerateTraceID() string {
	return fmt.Sprintf("tr_%d_%s", time.Now().Unix(), uuid.New().String()[:8])
}

// Contains checks if slice contains element
func Contains(slice []string, element string) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}

// ContainsInt checks if int slice contains element
func ContainsInt(slice []int, element int) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}

// ToPointer returns pointer to value
func ToPointer[T any](value T) *T {
	return &value
}

// FromPointer returns value from pointer or zero value if nil
func FromPointer[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

// FilterSlice filters slice based on predicate function
func FilterSlice[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// MapSlice transforms slice using mapper function
func MapSlice[T, R any](slice []T, mapper func(T) R) []R {
	result := make([]R, len(slice))
	for i, item := range slice {
		result[i] = mapper(item)
	}
	return result
}

// StringSliceToInterface converts string slice to interface slice
func StringSliceToInterface(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

// GetUserIDFromContext gets user ID from request context
func GetUserIDFromContext(r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(userIDKey).(string)
	return userID, ok
}

// GetUserRoleFromContext gets user role from request context
func GetUserRoleFromContext(r *http.Request) (string, bool) {
	role, ok := r.Context().Value(userRoleKey).(string)
	return role, ok
}

// GetUserEmailFromContext gets user email from request context
func GetUserEmailFromContext(r *http.Request) (string, bool) {
	email, ok := r.Context().Value(userEmailKey).(string)
	return email, ok
}

// GetUsernameFromContext gets username from request context
func GetUsernameFromContext(r *http.Request) (string, bool) {
	username, ok := r.Context().Value(usernameKey).(string)
	return username, ok
}

// SetRequestID sets request ID in context
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestIDFromContext extracts request ID from context
func GetRequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDKey).(string)
	return requestID, ok
}

// TimePointer returns pointer to time value
func TimePointer(t time.Time) *time.Time {
	return &t
}

// FormatCurrency formats price to Indonesian Rupiah format
func FormatCurrency(amount float64) string {
	return fmt.Sprintf("Rp %.0f", amount)
}

// TruncateString truncates string to specified length with ellipsis
func TruncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

// SanitizeString removes potentially harmful characters from string
func SanitizeString(s string) string {
	// Remove HTML tags and scripts
	s = strings.ReplaceAll(s, "<script>", "")
	s = strings.ReplaceAll(s, "</script>", "")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return strings.TrimSpace(s)
}

// CalculateDistance calculates distance between two coordinates (Haversine formula)
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371 // Earth radius in kilometers

	// Convert to radians
	lat1Rad := lat1 * (3.14159265359 / 180)
	lon1Rad := lon1 * (3.14159265359 / 180)
	lat2Rad := lat2 * (3.14159265359 / 180)
	lon2Rad := lon2 * (3.14159265359 / 180)

	// Differences
	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	// Haversine formula
	a := Sin(dLat/2)*Sin(dLat/2) + Cos(lat1Rad)*Cos(lat2Rad)*Sin(dLon/2)*Sin(dLon/2)
	c := 2 * Atan2(Sqrt(a), Sqrt(1-a))

	return earthRadius * c
}

// Helper math functions
func Sin(x float64) float64 {
	// Simple approximation for small angles
	return x - (x*x*x)/6 + (x*x*x*x*x)/120
}

func Cos(x float64) float64 {
	// Simple approximation
	return 1 - (x*x)/2 + (x*x*x*x)/24
}

func Sqrt(x float64) float64 {
	// Newton's method for square root
	if x <= 0 {
		return 0
	}

	guess := x / 2
	for i := 0; i < 10; i++ {
		guess = (guess + x/guess) / 2
	}
	return guess
}

func Atan2(y, x float64) float64 {
	// Simplified atan2 implementation
	if x > 0 {
		return Atan(y / x)
	}
	if x < 0 && y >= 0 {
		return Atan(y/x) + 3.14159265359
	}
	if x < 0 && y < 0 {
		return Atan(y/x) - 3.14159265359
	}
	if x == 0 && y > 0 {
		return 3.14159265359 / 2
	}
	if x == 0 && y < 0 {
		return -3.14159265359 / 2
	}
	return 0 // x == 0 && y == 0
}

func Atan(x float64) float64 {
	// Simple arctan approximation
	return x - (x*x*x)/3 + (x*x*x*x*x)/5
}

// GenerateUUID generates a new UUID string
func GenerateUUID() string {
	return uuid.New().String()
}

// ValidateStruct is a simple validation function (placeholder)
// TODO: Implement proper validation with error package
func ValidateStruct(s interface{}) error {
	// For now, just return nil (no validation)
	// This will be implemented properly when error package is fixed
	return nil
}

// ParseCoordinate parses a coordinate string to float64 with proper validation
func ParseCoordinate(value string) (float64, error) {
	// Trim whitespace
	value = strings.TrimSpace(value)

	// Check if empty
	if value == "" {
		return 0, fmt.Errorf("coordinate cannot be empty")
	}

	// Parse as float64
	coord, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid coordinate format: %s", value)
	}

	return coord, nil
}

// ValidateLatitude validates latitude value
func ValidateLatitude(latitude float64) error {
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("latitude must be between -90 and 90 degrees")
	}
	return nil
}

// ValidateLongitude validates longitude value
func ValidateLongitude(longitude float64) error {
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("longitude must be between -180 and 180 degrees")
	}
	return nil
}

// ParseAndValidateLatitude parses and validates latitude
func ParseAndValidateLatitude(value string) (float64, error) {
	coord, err := ParseCoordinate(value)
	if err != nil {
		return 0, err
	}

	if err := ValidateLatitude(coord); err != nil {
		return 0, err
	}

	return coord, nil
}

// ParseAndValidateLongitude parses and validates longitude
func ParseAndValidateLongitude(value string) (float64, error) {
	coord, err := ParseCoordinate(value)
	if err != nil {
		return 0, err
	}

	if err := ValidateLongitude(coord); err != nil {
		return 0, err
	}

	return coord, nil
}
