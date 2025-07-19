package models

import (
	"time"

	"github.com/google/uuid"
)

// FCMToken represents a Firebase Cloud Messaging token (Minimal Production Setup)
type FCMToken struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     string    `json:"user_id" db:"user_id"`
	Token      string    `json:"token" db:"token"`
	Platform   string    `json:"platform" db:"platform"` // android, ios, web
	IsActive   bool      `json:"is_active" db:"is_active"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
	LastUsedAt time.Time `json:"last_used_at" db:"last_used_at"`
}

// FCMTokenRequest represents a request to register/update FCM token (Minimal Setup)
type FCMTokenRequest struct {
	Token    string `json:"fcm_token" validate:"required"`
	Platform string `json:"platform" validate:"required,oneof=android ios web"`
}

// FCMTokenResponse represents FCM token response (Minimal Setup)
type FCMTokenResponse struct {
	ID        uuid.UUID `json:"id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToResponse converts FCMToken to FCMTokenResponse
func (f *FCMToken) ToResponse() *FCMTokenResponse {
	return &FCMTokenResponse{
		ID:        f.ID,
		Token:     f.Token,
		Platform:  f.Platform,
		IsActive:  f.IsActive,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

// IsValidPlatform checks if platform is valid
func IsValidPlatform(platform string) bool {
	validPlatforms := []string{"android", "ios", "web"}
	for _, p := range validPlatforms {
		if p == platform {
			return true
		}
	}
	return false
}
