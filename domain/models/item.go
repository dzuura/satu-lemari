package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Item represents a clothing item
type Item struct {
	ID                uuid.UUID `json:"id" db:"id"`
	PartnerID         string    `json:"partner_id" db:"partner_id"` // Firebase UID
	CategoryID        uuid.UUID `json:"category_id" db:"category_id"`
	Name              string    `json:"name" db:"name" validate:"required,min=2,max=255"`
	Description       *string   `json:"description,omitempty" db:"description"`
	Size              string    `json:"size" db:"size" validate:"required"`
	Color             *string   `json:"color,omitempty" db:"color"`
	Type              string    `json:"type" db:"type" validate:"required,oneof=donation rental thrifting"`
	Price             *float64  `json:"price,omitempty" db:"price"`
	TotalQuantity     int       `json:"total_quantity" db:"total_quantity" validate:"min=1"`
	AvailableQuantity int       `json:"available_quantity" db:"available_quantity"`
	Condition         string    `json:"condition" db:"condition" validate:"oneof=excellent good fair"`
	Images            []string  `json:"images" db:"images"`
	Status            string    `json:"status" db:"status" validate:"oneof=active inactive out_of_stock"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`

	// Relations (will be populated when needed)
	Partner  *UserProfile `json:"partner,omitempty"`
	Category *Category    `json:"category,omitempty"`
}

// CreateItemRequest represents request to create a new item
type CreateItemRequest struct {
	CategoryID    uuid.UUID `json:"category_id" validate:"required"`
	Name          string    `json:"name" validate:"required,min=2,max=255"`
	Description   *string   `json:"description,omitempty"`
	Size          string    `json:"size" validate:"required"`
	Color         *string   `json:"color,omitempty"`
	Type          string    `json:"type" validate:"required,oneof=donation rental thrifting"`
	Price         *float64  `json:"price,omitempty"`
	TotalQuantity int       `json:"total_quantity" validate:"min=1"`
	Condition     string    `json:"condition" validate:"oneof=excellent good fair"`
	Images        []string  `json:"images,omitempty"`
}

// UpdateItemRequest represents request to update an item
type UpdateItemRequest struct {
	CategoryID    *uuid.UUID `json:"category_id,omitempty"`
	Name          *string    `json:"name,omitempty" validate:"omitempty,min=2,max=255"`
	Description   *string    `json:"description,omitempty"`
	Size          *string    `json:"size,omitempty"`
	Color         *string    `json:"color,omitempty"`
	Price         *float64   `json:"price,omitempty"`
	TotalQuantity *int       `json:"total_quantity,omitempty" validate:"omitempty,min=1"`
	Condition     *string    `json:"condition,omitempty" validate:"omitempty,oneof=excellent good fair"`
	Images        []string   `json:"images,omitempty"`
	Status        *string    `json:"status,omitempty" validate:"omitempty,oneof=active inactive out_of_stock"`
}

// ItemFilter represents filters for item search
type ItemFilter struct {
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
	Type       *string    `json:"type,omitempty" validate:"omitempty,oneof=donation rental thrifting"`
	Size       *string    `json:"size,omitempty"`
	Color      *string    `json:"color,omitempty"`
	Condition  *string    `json:"condition,omitempty" validate:"omitempty,oneof=excellent good fair"`
	Status     *string    `json:"status,omitempty" validate:"omitempty,oneof=active inactive out_of_stock"`
	MinPrice   *float64   `json:"min_price,omitempty"`
	MaxPrice   *float64   `json:"max_price,omitempty"`
	Search     *string    `json:"search,omitempty"`
	PartnerID  *string    `json:"partner_id,omitempty"` // Firebase UID
	SortBy     *string    `json:"sort_by,omitempty" validate:"omitempty,oneof=name price created_at updated_at"`
	SortOrder  *string    `json:"sort_order,omitempty" validate:"omitempty,oneof=asc desc"`
}

// ItemWithDistance represents item with distance from user location
type ItemWithDistance struct {
	Item
	Distance *float64 `json:"distance,omitempty"` // in kilometers
}

// ItemStats represents item statistics
type ItemStats struct {
	TotalViews      int `json:"total_views"`
	TotalRequests   int `json:"total_requests"`
	TotalDonated    int `json:"total_donated"`
	TotalRented     int `json:"total_rented"`
	CurrentlyRented int `json:"currently_rented"`
}

// ItemWithStats represents item with statistics
type ItemWithStats struct {
	Item
	Stats ItemStats `json:"stats"`
}

// AIItemAnalysis represents AI analysis of item image
type AIItemAnalysis struct {
	DetectedCategory string            `json:"detected_category"`
	DetectedColor    string            `json:"detected_color"`
	DetectedSize     string            `json:"detected_size"`
	Confidence       float64           `json:"confidence"`
	SuggestedPrice   *float64          `json:"suggested_price,omitempty"`
	Tags             []string          `json:"tags"`
	Attributes       map[string]string `json:"attributes"`
}

// IsDonation checks if item is for donation
func (i *Item) IsDonation() bool {
	return i.Type == "donation"
}

// IsRental checks if item is for rental
func (i *Item) IsRental() bool {
	return i.Type == "rental"
}

// IsThrifting checks if item is for thrifting (buy/sell)
func (i *Item) IsThrifting() bool {
	return i.Type == "thrifting"
}

// IsAvailable checks if item is available for request
func (i *Item) IsAvailable() bool {
	return i.Status == "active" && i.AvailableQuantity > 0
}

// IsOutOfStock checks if item is out of stock
func (i *Item) IsOutOfStock() bool {
	return i.Status == "out_of_stock" || i.AvailableQuantity <= 0
}

// GetMainImage returns the first image or empty string
func (i *Item) GetMainImage() string {
	if len(i.Images) > 0 {
		return i.Images[0]
	}
	return ""
}

// GetFormattedPrice returns formatted price for rental/thrifting items
func (i *Item) GetFormattedPrice() string {
	if i.Price != nil && (i.IsRental() || i.IsThrifting()) {
		return FormatCurrency(*i.Price)
	}
	return "Free" // For donations
}

// FormatCurrency formats price to Indonesian Rupiah format
func FormatCurrency(amount float64) string {
	return fmt.Sprintf("Rp %.0f", amount)
}

// UpdateAvailableQuantity updates available quantity and status
func (i *Item) UpdateAvailableQuantity(quantity int) {
	i.AvailableQuantity = quantity
	if i.AvailableQuantity <= 0 {
		i.Status = "out_of_stock"
	} else if i.Status == "out_of_stock" {
		i.Status = "active"
	}
}
