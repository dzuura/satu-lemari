package storage

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// SupabaseStorage handles file uploads to Supabase Storage
type SupabaseStorage struct {
	config *config.Config
}

// UploadResult represents the result of a file upload
type UploadResult struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// NewSupabaseStorage creates a new Supabase storage service
func NewSupabaseStorage(cfg *config.Config) *SupabaseStorage {
	return &SupabaseStorage{
		config: cfg,
	}
}

// UploadFile uploads a single file to Supabase Storage
func (s *SupabaseStorage) UploadFile(file *multipart.FileHeader, bucket string, folder string) (*UploadResult, *appError.AppError) {
	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to open uploaded file")
	}
	defer src.Close()

	// Read file content
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read file content")
	}

	// Validate file size (max 10MB)
	if len(fileBytes) > 10*1024*1024 {
		return nil, appError.New(appError.ErrInvalidInput, "File size too large. Maximum size is 10MB")
	}

	// Validate file type
	if !s.isValidImageType(file.Header.Get("Content-Type")) {
		return nil, appError.New(appError.ErrInvalidInput, "Invalid file type. Only images are allowed")
	}

	// Generate unique filename
	filename := s.generateFilename(file.Filename, folder)
	filePath := fmt.Sprintf("%s/%s", folder, filename)

	// Upload to Supabase Storage
	url, err := s.uploadToSupabase(fileBytes, bucket, filePath, file.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("Failed to upload file to Supabase: %v", err)
		return nil, appError.New(appError.ErrInternal, "Failed to upload file")
	}

	return &UploadResult{
		URL:      url,
		Path:     filePath,
		Size:     int64(len(fileBytes)),
		MimeType: file.Header.Get("Content-Type"),
	}, nil
}

// UploadMultipleFiles uploads multiple files to Supabase Storage
func (s *SupabaseStorage) UploadMultipleFiles(files []*multipart.FileHeader, bucket string, folder string) ([]*UploadResult, *appError.AppError) {
	var results []*UploadResult

	for _, file := range files {
		result, err := s.UploadFile(file, bucket, folder)
		if err != nil {
			// If one file fails, we should clean up previously uploaded files
			// For now, we'll return the error and let the caller handle cleanup
			return results, err
		}
		results = append(results, result)
	}

	return results, nil
}

// DeleteFile deletes a file from Supabase Storage
func (s *SupabaseStorage) DeleteFile(bucket string, filePath string) *appError.AppError {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.config.SupabaseURL, bucket, filePath)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create delete request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to delete file")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to delete file from Supabase: %s", string(bodyBytes))
		return appError.New(appError.ErrInternal, "Failed to delete file from storage")
	}

	return nil
}

// uploadToSupabase uploads file content to Supabase Storage
func (s *SupabaseStorage) uploadToSupabase(fileBytes []byte, bucket string, filePath string, contentType string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.config.SupabaseURL, bucket, filePath)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(fileBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Return the public URL
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.config.SupabaseURL, bucket, filePath)
	return publicURL, nil
}

// generateFilename generates a unique filename
func (s *SupabaseStorage) generateFilename(originalName string, folder string) string {
	ext := filepath.Ext(originalName)
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s_%d%s", folder, timestamp, ext)
}

// isValidImageType checks if the content type is a valid image
func (s *SupabaseStorage) isValidImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
	}

	for _, validType := range validTypes {
		if strings.EqualFold(contentType, validType) {
			return true
		}
	}
	return false
}

// GetPublicURL returns the public URL for a file
func (s *SupabaseStorage) GetPublicURL(bucket string, filePath string) string {
	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.config.SupabaseURL, bucket, filePath)
}
