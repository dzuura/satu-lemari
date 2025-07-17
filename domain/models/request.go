package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Request represents a donation or rental request
type Request struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	ItemID           uuid.UUID  `json:"item_id" db:"item_id"`
	UserID           string     `json:"user_id" db:"user_id"`       // Firebase UID
	PartnerID        string     `json:"partner_id" db:"partner_id"` // Firebase UID
	Type             string     `json:"type" db:"type" validate:"required,oneof=donation rental"`
	Quantity         int        `json:"quantity" db:"quantity" validate:"min=1"`
	Reason           *string    `json:"reason,omitempty" db:"reason"`
	ContactInfo      *string    `json:"contact_info,omitempty" db:"contact_info"`
	PickupDate       *time.Time `json:"pickup_date,omitempty" db:"pickup_date"`
	ReturnDate       *time.Time `json:"return_date,omitempty" db:"return_date"`
	Status           string     `json:"status" db:"status" validate:"oneof=pending approved rejected completed returned"`
	RejectionReason  *string    `json:"rejection_reason,omitempty" db:"rejection_reason"`
	QueuePosition    *int       `json:"queue_position,omitempty" db:"queue_position"`
	PriorityScore    float64    `json:"priority_score" db:"priority_score"`
	DeletedByUser    bool       `json:"deleted_by_user" db:"deleted_by_user"`
	DeletedByPartner bool       `json:"deleted_by_partner" db:"deleted_by_partner"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`

	// Relations (will be populated when needed)
	Item    *Item        `json:"item,omitempty"`
	User    *UserProfile `json:"user,omitempty"`
	Partner *UserProfile `json:"partner,omitempty"`

	// Additional fields for API responses
	ItemName     string   `json:"item_name,omitempty"`
	ItemPrice    *float64 `json:"item_price,omitempty"`    // For rental type only
	ItemImages   []string `json:"item_images,omitempty"`   // Item images
	CategoryName string   `json:"category_name,omitempty"` // Category name
	UserName     string   `json:"user_name,omitempty"`
	UserFullName string   `json:"user_full_name,omitempty"`
	UserPhone    *string  `json:"user_phone,omitempty"`
	UserPhoto    *string  `json:"user_photo"`
}

// CreateRequestRequest represents request to create a new request
type CreateRequestRequest struct {
	ItemID      uuid.UUID  `json:"item_id" validate:"required"`
	Quantity    int        `json:"quantity" validate:"min=1"`
	Reason      *string    `json:"reason,omitempty"`
	ContactInfo *string    `json:"contact_info,omitempty"`
	PickupDate  *time.Time `json:"pickup_date,omitempty"`
	ReturnDate  *time.Time `json:"return_date,omitempty"` // For rental only
}

// UpdateRequestRequest represents request to update a request
type UpdateRequestRequest struct {
	Status          *string    `json:"status,omitempty" validate:"omitempty,oneof=pending approved rejected completed returned"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	PickupDate      *time.Time `json:"pickup_date,omitempty"`
	ReturnDate      *time.Time `json:"return_date,omitempty"`
}

// RequestFilter represents filters for request search
type RequestFilter struct {
	Type      *string    `json:"type,omitempty" validate:"omitempty,oneof=donation rental"`
	Status    *string    `json:"status,omitempty" validate:"omitempty,oneof=pending approved rejected completed returned"`
	UserID    *string    `json:"user_id,omitempty"`    // Firebase UID
	PartnerID *string    `json:"partner_id,omitempty"` // Firebase UID
	ItemID    *uuid.UUID `json:"item_id,omitempty"`
	DateFrom  *time.Time `json:"date_from,omitempty"`
	DateTo    *time.Time `json:"date_to,omitempty"`
	SortBy    *string    `json:"sort_by,omitempty" validate:"omitempty,oneof=created_at updated_at priority_score"`
	SortOrder *string    `json:"sort_order,omitempty" validate:"omitempty,oneof=asc desc"`
}

// RequestStats represents request statistics
type RequestStats struct {
	TotalRequests     int `json:"total_requests"`
	PendingRequests   int `json:"pending_requests"`
	ApprovedRequests  int `json:"approved_requests"`
	RejectedRequests  int `json:"rejected_requests"`
	CompletedRequests int `json:"completed_requests"`
	DonationRequests  int `json:"donation_requests"`
	RentalRequests    int `json:"rental_requests"`
}

// RequestWithQueue represents request with queue information
type RequestWithQueue struct {
	Request
	QueueInfo *QueueInfo `json:"queue_info,omitempty"`
}

// QueueInfo represents queue information for a request
type QueueInfo struct {
	Position       int    `json:"position"`
	TotalInQueue   int    `json:"total_in_queue"`
	EstimatedWait  string `json:"estimated_wait"`
	CanBeProcessed bool   `json:"can_be_processed"`
}

// IsDonation checks if request is for donation
func (r *Request) IsDonation() bool {
	return r.Type == "donation"
}

// IsRental checks if request is for rental
func (r *Request) IsRental() bool {
	return r.Type == "rental"
}

// IsPending checks if request is pending
func (r *Request) IsPending() bool {
	return r.Status == "pending"
}

// IsApproved checks if request is approved
func (r *Request) IsApproved() bool {
	return r.Status == "approved"
}

// IsRejected checks if request is rejected
func (r *Request) IsRejected() bool {
	return r.Status == "rejected"
}

// IsCompleted checks if request is completed
func (r *Request) IsCompleted() bool {
	return r.Status == "completed"
}

// IsReturned checks if rental request is returned
func (r *Request) IsReturned() bool {
	return r.Status == "returned"
}

// CanBeApproved checks if request can be approved
func (r *Request) CanBeApproved() bool {
	return r.Status == "pending"
}

// CanBeRejected checks if request can be rejected
func (r *Request) CanBeRejected() bool {
	return r.Status == "pending"
}

// CanBeCompleted checks if request can be marked as completed
func (r *Request) CanBeCompleted() bool {
	return r.Status == "approved"
}

// CanBeReturned checks if rental request can be marked as returned
func (r *Request) CanBeReturned() bool {
	return r.IsRental() && r.Status == "approved"
}

// IsOverdue checks if rental request is overdue
func (r *Request) IsOverdue() bool {
	if !r.IsRental() || r.ReturnDate == nil {
		return false
	}

	now := time.Now()
	return now.After(*r.ReturnDate) && !r.IsReturned() && !r.IsCompleted()
}

// GetDaysUntilReturn calculates days until return date (for rentals)
func (r *Request) GetDaysUntilReturn() *int {
	if !r.IsRental() || r.ReturnDate == nil {
		return nil
	}

	now := time.Now()
	days := int(r.ReturnDate.Sub(now).Hours() / 24)
	return &days
}

// GetTotalCost calculates total cost for rental requests
func (r *Request) GetTotalCost() *float64 {
	if !r.IsRental() || r.Item == nil || r.Item.Price == nil {
		return nil
	}

	// Calculate days if return date is set
	if r.ReturnDate != nil && r.PickupDate != nil {
		days := int(r.ReturnDate.Sub(*r.PickupDate).Hours()/24) + 1 // 1 for partial day
		if days < 1 {
			days = 1
		}
		cost := *r.Item.Price * float64(days) * float64(r.Quantity)
		return &cost
	}

	// Default to single day cost
	cost := *r.Item.Price * float64(r.Quantity)
	return &cost
}

// GetFormattedTotalCost returns formatted total cost
func (r *Request) GetFormattedTotalCost() string {
	cost := r.GetTotalCost()
	if cost == nil {
		return "Free"
	}
	return FormatCurrency(*cost)
}

// GetStatusLabel returns user-friendly status label
func (r *Request) GetStatusLabel() string {
	switch r.Status {
	case "pending":
		return "Menunggu Persetujuan"
	case "approved":
		return "Disetujui"
	case "rejected":
		return "Ditolak"
	case "completed":
		return "Selesai"
	case "returned":
		return "Dikembalikan"
	default:
		return "Unknown"
	}
}

// GetTypeLabel returns user-friendly type label
func (r *Request) GetTypeLabel() string {
	switch r.Type {
	case "donation":
		return "Donasi"
	case "rental":
		return "Sewa"
	default:
		return "Unknown"
	}
}

// UnmarshalJSON custom unmarshaling to handle date fields from Supabase
func (r *Request) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with string fields for dates
	type Alias Request
	aux := &struct {
		PickupDate *string `json:"pickup_date"`
		ReturnDate *string `json:"return_date"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse pickup_date if not null
	if aux.PickupDate != nil && *aux.PickupDate != "" {
		if t, err := time.Parse("2006-01-02", *aux.PickupDate); err == nil {
			r.PickupDate = &t
		}
	}

	// Parse return_date if not null
	if aux.ReturnDate != nil && *aux.ReturnDate != "" {
		if t, err := time.Parse("2006-01-02", *aux.ReturnDate); err == nil {
			r.ReturnDate = &t
		}
	}

	return nil
}

// UnmarshalJSON custom unmarshaling for UpdateRequestRequest to handle date fields
func (u *UpdateRequestRequest) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with string fields for dates
	type Alias UpdateRequestRequest
	aux := &struct {
		PickupDate *string `json:"pickup_date"`
		ReturnDate *string `json:"return_date"`
		*Alias
	}{
		Alias: (*Alias)(u),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse pickup_date if provided
	if aux.PickupDate != nil && *aux.PickupDate != "" {
		if t, err := time.Parse("2006-01-02", *aux.PickupDate); err == nil {
			u.PickupDate = &t
		} else {
			return fmt.Errorf("invalid pickup_date format, expected YYYY-MM-DD")
		}
	}

	// Parse return_date if provided
	if aux.ReturnDate != nil && *aux.ReturnDate != "" {
		if t, err := time.Parse("2006-01-02", *aux.ReturnDate); err == nil {
			u.ReturnDate = &t
		} else {
			return fmt.Errorf("invalid return_date format, expected YYYY-MM-DD")
		}
	}

	return nil
}
