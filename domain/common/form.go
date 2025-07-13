package common

import (
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

// ParseFormData parses multipart form data and returns form values and files
func ParseFormData(r *http.Request) (map[string]string, map[string][]*multipart.FileHeader, error) {
	// Parse multipart form (max 32MB)
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		return nil, nil, err
	}

	// Extract form values
	formValues := make(map[string]string)
	for key, values := range r.Form {
		if len(values) > 0 {
			formValues[key] = values[0]
		}
	}

	// Extract files
	files := make(map[string][]*multipart.FileHeader)
	for key, fileHeaders := range r.MultipartForm.File {
		files[key] = fileHeaders
	}

	return formValues, files, nil
}

// GetFormValue gets a string value from form data
func GetFormValue(formValues map[string]string, key string) string {
	return formValues[key]
}

// GetFormValueOrDefault gets a string value from form data with default
func GetFormValueOrDefault(formValues map[string]string, key string, defaultValue string) string {
	if value, exists := formValues[key]; exists && value != "" {
		return value
	}
	return defaultValue
}

// GetFormInt gets an integer value from form data
func GetFormInt(formValues map[string]string, key string) (int, error) {
	value := formValues[key]
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

// GetFormIntOrDefault gets an integer value from form data with default
func GetFormIntOrDefault(formValues map[string]string, key string, defaultValue int) int {
	value, err := GetFormInt(formValues, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetFormFloat gets a float value from form data
func GetFormFloat(formValues map[string]string, key string) (float64, error) {
	value := formValues[key]
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

// GetFormFloatOrDefault gets a float value from form data with default
func GetFormFloatOrDefault(formValues map[string]string, key string, defaultValue float64) float64 {
	value, err := GetFormFloat(formValues, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetFormBool gets a boolean value from form data
func GetFormBool(formValues map[string]string, key string) (bool, error) {
	value := formValues[key]
	if value == "" {
		return false, nil
	}
	return strconv.ParseBool(strings.ToLower(value))
}

// GetFormBoolOrDefault gets a boolean value from form data with default
func GetFormBoolOrDefault(formValues map[string]string, key string, defaultValue bool) bool {
	value, err := GetFormBool(formValues, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetFormFiles gets files from form data
func GetFormFiles(files map[string][]*multipart.FileHeader, key string) []*multipart.FileHeader {
	if fileHeaders, exists := files[key]; exists {
		return fileHeaders
	}
	return nil
}

// GetFormFile gets a single file from form data
func GetFormFile(files map[string][]*multipart.FileHeader, key string) *multipart.FileHeader {
	fileHeaders := GetFormFiles(files, key)
	if len(fileHeaders) > 0 {
		return fileHeaders[0]
	}
	return nil
}

// ValidateRequiredFormFields validates that required fields are present
func ValidateRequiredFormFields(formValues map[string]string, requiredFields []string) []string {
	var missingFields []string
	for _, field := range requiredFields {
		if value := formValues[field]; value == "" {
			missingFields = append(missingFields, field)
		}
	}
	return missingFields
}

// ValidateRequiredFiles validates that required files are present
func ValidateRequiredFiles(files map[string][]*multipart.FileHeader, requiredFiles []string) []string {
	var missingFiles []string
	for _, fileKey := range requiredFiles {
		if fileHeaders := files[fileKey]; len(fileHeaders) == 0 {
			missingFiles = append(missingFiles, fileKey)
		}
	}
	return missingFiles
}
