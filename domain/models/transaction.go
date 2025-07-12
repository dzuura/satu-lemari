package models

import (
	"time"

	"github.com/google/uuid"
)

// Transaction represents a completed donation or rental transaction
type Transaction struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	RequestID        uuid.UUID  `json:"request_id" db:"request_id"`
	ItemID           uuid.UUID  `json:"item_id" db:"item_id"`
	UserID           string     `json:"user_id" db:"user_id"`       // Firebase UID
	PartnerID        string     `json:"partner_id" db:"partner_id"` // Firebase UID
	Type             string     `json:"type" db:"type" validate:"required,oneof=donation rental"`
	Quantity         int        `json:"quantity" db:"quantity" validate:"min=1"`
	Amount           float64    `json:"amount" db:"amount"`
	Status           string     `json:"status" db:"status" validate:"oneof=active completed cancelled"`
	PickupDate       *time.Time `json:"pickup_date,omitempty" db:"pickup_date"`
	ReturnDate       *time.Time `json:"return_date,omitempty" db:"return_date"`
	ActualReturnDate *time.Time `json:"actual_return_date,omitempty" db:"actual_return_date"`
	Rating           *int       `json:"rating,omitempty" db:"rating" validate:"omitempty,min=1,max=5"`
	Review           *string    `json:"review,omitempty" db:"review"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`

	// Relations (will be populated when needed)
	Request *Request     `json:"request,omitempty"`
	Item    *Item        `json:"item,omitempty"`
	User    *UserProfile `json:"user,omitempty"`
	Partner *UserProfile `json:"partner,omitempty"`
}

// CreateTransactionRequest represents request to create a new transaction
type CreateTransactionRequest struct {
	RequestID  uuid.UUID  `json:"request_id" validate:"required"`
	PickupDate *time.Time `json:"pickup_date,omitempty"`
	ReturnDate *time.Time `json:"return_date,omitempty"`
}

// UpdateTransactionRequest represents request to update a transaction
type UpdateTransactionRequest struct {
	Status           *string    `json:"status,omitempty" validate:"omitempty,oneof=active completed cancelled"`
	ActualReturnDate *time.Time `json:"actual_return_date,omitempty"`
	Rating           *int       `json:"rating,omitempty" validate:"omitempty,min=1,max=5"`
	Review           *string    `json:"review,omitempty"`
}

// TransactionFilter represents filters for transaction search
type TransactionFilter struct {
	Type      *string    `json:"type,omitempty" validate:"omitempty,oneof=donation rental"`
	Status    *string    `json:"status,omitempty" validate:"omitempty,oneof=active completed cancelled"`
	UserID    *string    `json:"user_id,omitempty"`    // Firebase UID
	PartnerID *string    `json:"partner_id,omitempty"` // Firebase UID
	ItemID    *uuid.UUID `json:"item_id,omitempty"`
	DateFrom  *time.Time `json:"date_from,omitempty"`
	DateTo    *time.Time `json:"date_to,omitempty"`
	MinAmount *float64   `json:"min_amount,omitempty"`
	MaxAmount *float64   `json:"max_amount,omitempty"`
	SortBy    *string    `json:"sort_by,omitempty" validate:"omitempty,oneof=created_at amount rating"`
	SortOrder *string    `json:"sort_order,omitempty" validate:"omitempty,oneof=asc desc"`
}

// TransactionStats represents transaction statistics
type TransactionStats struct {
	TotalTransactions     int     `json:"total_transactions"`
	TotalAmount           float64 `json:"total_amount"`
	TotalDonations        int     `json:"total_donations"`
	TotalRentals          int     `json:"total_rentals"`
	CompletedTransactions int     `json:"completed_transactions"`
	ActiveTransactions    int     `json:"active_transactions"`
	AverageRating         float64 `json:"average_rating"`
	TotalReviews          int     `json:"total_reviews"`
}

// IsDonation checks if transaction is for donation
func (t *Transaction) IsDonation() bool {
	return t.Type == "donation"
}

// IsRental checks if transaction is for rental
func (t *Transaction) IsRental() bool {
	return t.Type == "rental"
}

// IsActive checks if transaction is active
func (t *Transaction) IsActive() bool {
	return t.Status == "active"
}

// IsCompleted checks if transaction is completed
func (t *Transaction) IsCompleted() bool {
	return t.Status == "completed"
}

// IsCancelled checks if transaction is cancelled
func (t *Transaction) IsCancelled() bool {
	return t.Status == "cancelled"
}

// IsOverdue checks if rental transaction is overdue
func (t *Transaction) IsOverdue() bool {
	if !t.IsRental() || t.ReturnDate == nil || t.ActualReturnDate != nil {
		return false
	}

	now := time.Now()
	return now.After(*t.ReturnDate)
}

// CanBeCompleted checks if transaction can be marked as completed
func (t *Transaction) CanBeCompleted() bool {
	return t.Status == "active"
}

// CanBeCancelled checks if transaction can be cancelled
func (t *Transaction) CanBeCancelled() bool {
	return t.Status == "active"
}

// CanBeReturned checks if rental transaction can be marked as returned
func (t *Transaction) CanBeReturned() bool {
	return t.IsRental() && t.Status == "active" && t.ActualReturnDate == nil
}

// HasReview checks if transaction has a review
func (t *Transaction) HasReview() bool {
	return t.Review != nil && *t.Review != ""
}

// HasRating checks if transaction has a rating
func (t *Transaction) HasRating() bool {
	return t.Rating != nil
}

// GetDaysUntilReturn calculates days until return date (for rentals)
func (t *Transaction) GetDaysUntilReturn() *int {
	if !t.IsRental() || t.ReturnDate == nil || t.ActualReturnDate != nil {
		return nil
	}

	now := time.Now()
	days := int(t.ReturnDate.Sub(now).Hours() / 24)
	return &days
}

// GetRentalDuration calculates rental duration in days
func (t *Transaction) GetRentalDuration() *int {
	if !t.IsRental() || t.PickupDate == nil || t.ReturnDate == nil {
		return nil
	}

	days := int(t.ReturnDate.Sub(*t.PickupDate).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	return &days
}

// GetFormattedAmount returns formatted transaction amount
func (t *Transaction) GetFormattedAmount() string {
	if t.Amount == 0 {
		return "Free"
	}
	return FormatCurrency(t.Amount)
}

// GetStatusLabel returns user-friendly status label
func (t *Transaction) GetStatusLabel() string {
	switch t.Status {
	case "active":
		return "Aktif"
	case "completed":
		return "Selesai"
	case "cancelled":
		return "Dibatalkan"
	default:
		return "Unknown"
	}
}

// GetTypeLabel returns user-friendly type label
func (t *Transaction) GetTypeLabel() string {
	switch t.Type {
	case "donation":
		return "Donasi"
	case "rental":
		return "Sewa"
	default:
		return "Unknown"
	}
}

// GetRatingStars returns rating as star representation
func (t *Transaction) GetRatingStars() string {
	if t.Rating == nil {
		return "No rating"
	}

	stars := ""
	for i := 1; i <= 5; i++ {
		if i <= *t.Rating {
			stars = "★"
		} else {
			stars = "☆"
		}
	}
	return stars
}
