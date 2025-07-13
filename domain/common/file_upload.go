package common

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// FileUploadService handles file uploads to Supabase Storage
type FileUploadService struct {
	supabaseURL string
	supabaseKey string
}

// NewFileUploadService creates a new file upload service
func NewFileUploadService(supabaseURL, supabaseKey string) *FileUploadService {
	return &FileUploadService{
		supabaseURL: supabaseURL,
		supabaseKey: supabaseKey,
	}
}

// UploadUserPhoto uploads a user photo to Supabase Storage
func (s *FileUploadService) UploadUserPhoto(userID string, file *multipart.FileHeader) (string, error) {
	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %v", err)
	}
	defer src.Close()

	// Read file content
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %v", err)
	}

	// Generate unique filename
	ext := filepath.Ext(file.Filename)
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%s_%d%s", userID, timestamp, ext)

	// Upload to Supabase Storage using the correct REST API
	url := fmt.Sprintf("%s/storage/v1/object/user-photos/%s", s.supabaseURL, filename)

	req, err := http.NewRequest("POST", url, bytes.NewReader(fileBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
	req.Header.Set("Content-Type", file.Header.Get("Content-Type"))
	req.Header.Set("Cache-Control", "3600")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("bucket 'user-photos' not found. Please create it manually in Supabase Dashboard or run the SQL command provided in the documentation")
		}
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Construct public URL
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/user-photos/%s", s.supabaseURL, filename)
	return publicURL, nil
}

// DeleteUserPhoto deletes a user photo from Supabase Storage
func (s *FileUploadService) DeleteUserPhoto(photoURL string) error {
	// Extract filename from URL
	parts := strings.Split(photoURL, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid photo URL")
	}
	filename := parts[len(parts)-1]

	// Use the correct delete API
	url := fmt.Sprintf("%s/storage/v1/object/user-photos/%s", s.supabaseURL, filename)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.supabaseKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ValidateImageFile validates if the uploaded file is a valid image
func ValidateImageFile(file *multipart.FileHeader) error {
	// Check file size (max 5MB)
	if file.Size > 5*1024*1024 {
		return fmt.Errorf("file size too large, maximum 5MB allowed")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}

	isAllowed := false
	for _, allowedExt := range allowedExts {
		if ext == allowedExt {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return fmt.Errorf("invalid file type, only JPG, PNG, GIF, and WebP are allowed")
	}

	// Check MIME type
	contentType := file.Header.Get("Content-Type")
	allowedMimes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
	}

	isValidMime := false
	for _, allowedMime := range allowedMimes {
		if contentType == allowedMime {
			isValidMime = true
			break
		}
	}

	if !isValidMime {
		return fmt.Errorf("invalid MIME type")
	}

	return nil
}
