package models

import (
	"time"
	"github.com/google/uuid"
)

// Category represents a clothing category
type Category struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Name        string    `json:"name" db:"name" validate:"required,min=2,max=100"`
	Description *string   `json:"description,omitempty" db:"description"`
	Icon        *string   `json:"icon,omitempty" db:"icon"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateCategoryRequest represents request to create a new category
type CreateCategoryRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=100"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

// UpdateCategoryRequest represents request to update a category
type UpdateCategoryRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// CategoryWithStats represents category with item statistics
type CategoryWithStats struct {
	Category
	TotalItems     int `json:"total_items"`
	ActiveItems    int `json:"active_items"`
	DonationItems  int `json:"donation_items"`
	RentalItems    int `json:"rental_items"`
}