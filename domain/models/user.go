package models

import (
	"time"
	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	Email               string     `json:"email" db:"email" validate:"required,email"`
	Username            string     `json:"username" db:"username" validate:"required,min=3,max=30"`
	FullName            *string    `json:"full_name,omitempty" db:"full_name"`
	Role                string     `json:"role" db:"role" validate:"required,oneof=user partner admin"`
	Phone               *string    `json:"phone,omitempty" db:"phone"`
	Address             *string    `json:"address,omitempty" db:"address"`
	City                *string    `json:"city,omitempty" db:"city"`
	Latitude            *float64   `json:"latitude,omitempty" db:"latitude"`
	Longitude           *float64   `json:"longitude,omitempty" db:"longitude"`
	Photo               *string    `json:"photo,omitempty" db:"photo"`
	Description         *string    `json:"description,omitempty" db:"description"`
	IsActive            bool       `json:"is_active" db:"is_active"`
	WeeklyDonationQuota int        `json:"weekly_donation_quota" db:"weekly_donation_quota"`
	WeeklyDonationUsed  int        `json:"weekly_donation_used" db:"weekly_donation_used"`
	QuotaResetDate      time.Time  `json:"quota_reset_date" db:"quota_reset_date"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// CreateUserRequest represents request to create a new user
type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,min=3,max=30"`
	FullName string `json:"full_name,omitempty"`
	Role     string `json:"role" validate:"required,oneof=user partner admin"`
	Phone    string `json:"phone,omitempty"`
}

// UpdateUserRequest represents request to update user profile
type UpdateUserRequest struct {
	FullName    *string  `json:"full_name,omitempty"`
	Phone       *string  `json:"phone,omitempty"`
	Address     *string  `json:"address,omitempty"`
	City        *string  `json:"city,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Photo       *string  `json:"photo,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// UpdateLocationRequest represents request to update user location
type UpdateLocationRequest struct {
	Latitude  float64 `json:"latitude" validate:"required,min=-90,max=90"`
	Longitude float64 `json:"longitude" validate:"required,min=-180,max=180"`
	Address   string  `json:"address,omitempty"`
	City      string  `json:"city,omitempty"`
}

// UserProfile represents public user profile information
type UserProfile struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	FullName    *string   `json:"full_name,omitempty"`
	Photo       *string   `json:"photo,omitempty"`
	City        *string   `json:"city,omitempty"`
	Description *string   `json:"description,omitempty"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserWithLocation represents user with location for distance calculations
type UserWithLocation struct {
	User
	Distance *float64 `json:"distance,omitempty"` // in kilometers
}

// UserStats represents user statistics for dashboard
type UserStats struct {
	TotalDonations     int `json:"total_donations"`
	TotalRentals       int `json:"total_rentals"`
	ActiveItems        int `json:"active_items"`
	PendingRequests    int `json:"pending_requests"`
	CompletedRequests  int `json:"completed_requests"`
	WeeklyQuotaUsed    int `json:"weekly_quota_used"`
	WeeklyQuotaRemaining int `json:"weekly_quota_remaining"`
}

// UserDashboard represents dashboard data for partner users
type UserDashboard struct {
	Stats           UserStats `json:"stats"`
	RecentRequests  []Request `json:"recent_requests"`
	RecentItems     []Item    `json:"recent_items"`
}

// IsPartner checks if user is a partner
func (u *User) IsPartner() bool {
	return u.Role == "partner"
}

// IsAdmin checks if user is an admin
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// IsUser checks if user is a regular user
func (u *User) IsUser() bool {
	return u.Role == "user"
}

// CanRequestDonation checks if user can request donations based on quota
func (u *User) CanRequestDonation() bool {
	if u.Role != "user" {
		return false
	}
	
	// Check if quota needs reset (weekly)
	now := time.Now()
	if now.Sub(u.QuotaResetDate).Hours() >= 168 { // 7 days * 24 hours
		return true // Quota should be reset
	}
	
	return u.WeeklyDonationUsed < u.WeeklyDonationQuota
}

// GetRemainingDonationQuota returns remaining donation quota
func (u *User) GetRemainingDonationQuota() int {
	remaining := u.WeeklyDonationQuota - u.WeeklyDonationUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// HasLocation checks if user has location set
func (u *User) HasLocation() bool {
	return u.Latitude != nil && u.Longitude != nil
}

// GetDisplayName returns the display name (full name or username)
func (u *User) GetDisplayName() string {
	if u.FullName != nil && *u.FullName != "" {
		return *u.FullName
	}
	return u.Username
}

// ToProfile converts User to UserProfile (public info only)
func (u *User) ToProfile() UserProfile {
	return UserProfile{
		ID:          u.ID,
		Username:    u.Username,
		FullName:    u.FullName,
		Photo:       u.Photo,
		City:        u.City,
		Description: u.Description,
		Role:        u.Role,
		CreatedAt:   u.CreatedAt,
	}
}