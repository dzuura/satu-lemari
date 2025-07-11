package models

import (
	"fmt"
	"time"
	"github.com/google/uuid"
)

// Notification represents a system notification
type Notification struct {
	ID        uuid.UUID              `json:"id" db:"id"`
	UserID    uuid.UUID              `json:"user_id" db:"user_id"`
	Title     string                 `json:"title" db:"title" validate:"required,max=255"`
	Message   string                 `json:"message" db:"message" validate:"required"`
	Type      string                 `json:"type" db:"type" validate:"required"`
	RelatedID *uuid.UUID             `json:"related_id,omitempty" db:"related_id"`
	Data      map[string]interface{} `json:"data,omitempty" db:"data"`
	IsRead    bool                   `json:"is_read" db:"is_read"`
	IsSent    bool                   `json:"is_sent" db:"is_sent"`
	Platform  string                 `json:"platform" db:"platform" validate:"oneof=web mobile both"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	ReadAt    *time.Time             `json:"read_at,omitempty" db:"read_at"`
}

// NotificationType represents notification types
type NotificationType string

const (
	// Request related notifications
	NotifRequestCreated   NotificationType = "request_created"
	NotifRequestApproved  NotificationType = "request_approved"
	NotifRequestRejected  NotificationType = "request_rejected"
	NotifRequestCompleted NotificationType = "request_completed"
	NotifRequestReturned  NotificationType = "request_returned"
	
	// Item related notifications
	NotifItemAdded       NotificationType = "item_added"
	NotifItemUpdated     NotificationType = "item_updated"
	NotifItemOutOfStock  NotificationType = "item_out_of_stock"
	NotifItemAvailable   NotificationType = "item_available"
	
	// System notifications
	NotifQuotaReset      NotificationType = "quota_reset"
	NotifQuotaExceeded   NotificationType = "quota_exceeded"
	NotifReminderReturn  NotificationType = "reminder_return"
	NotifOverdueItem     NotificationType = "overdue_item"
	
	// General notifications
	NotifWelcome         NotificationType = "welcome"
	NotifSystemUpdate    NotificationType = "system_update"
	NotifPromotion       NotificationType = "promotion"
)

// CreateNotificationRequest represents request to create a notification
type CreateNotificationRequest struct {
	UserID    uuid.UUID              `json:"user_id" validate:"required"`
	Title     string                 `json:"title" validate:"required,max=255"`
	Message   string                 `json:"message" validate:"required"`
	Type      string                 `json:"type" validate:"required"`
	RelatedID *uuid.UUID             `json:"related_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Platform  string                 `json:"platform" validate:"oneof=web mobile both"`
}

// NotificationFilter represents filters for notification search
type NotificationFilter struct {
	UserID   *uuid.UUID `json:"user_id,omitempty"`
	Type     *string    `json:"type,omitempty"`
	IsRead   *bool      `json:"is_read,omitempty"`
	Platform *string    `json:"platform,omitempty" validate:"omitempty,oneof=web mobile both"`
	DateFrom *time.Time `json:"date_from,omitempty"`
	DateTo   *time.Time `json:"date_to,omitempty"`
}

// NotificationStats represents notification statistics
type NotificationStats struct {
	TotalNotifications int `json:"total_notifications"`
	UnreadCount        int `json:"unread_count"`
	ReadCount          int `json:"read_count"`
	TodayCount         int `json:"today_count"`
	WeekCount          int `json:"week_count"`
}

// MarkAsReadRequest represents request to mark notifications as read
type MarkAsReadRequest struct {
	NotificationIDs []uuid.UUID `json:"notification_ids" validate:"required,min=1"`
}

// IsUnread checks if notification is unread
func (n *Notification) IsUnread() bool {
	return !n.IsRead
}

// MarkAsRead marks notification as read
func (n *Notification) MarkAsRead() {
	n.IsRead = true
	now := time.Now()
	n.ReadAt = &now
}

// GetTimeAgo returns time ago string for notification
func (n *Notification) GetTimeAgo() string {
	now := time.Now()
	diff := now.Sub(n.CreatedAt)
	
	if diff.Hours() < 1 {
		minutes := int(diff.Minutes())
		if minutes < 1 {
			return "Baru saja"
		}
		return fmt.Sprintf("%d menit yang lalu", minutes)
	}
	
	if diff.Hours() < 24 {
		hours := int(diff.Hours())
		return fmt.Sprintf("%d jam yang lalu", hours)
	}
	
	days := int(diff.Hours() / 24)
	if days == 1 {
		return "Kemarin"
	}
	
	if days < 7 {
		return fmt.Sprintf("%d hari yang lalu", days)
	}
	
	weeks := days / 7
	if weeks == 1 {
		return "Seminggu yang lalu"
	}
	
	if weeks < 4 {
		return fmt.Sprintf("%d minggu yang lalu", weeks)
	}
	
	return n.CreatedAt.Format("2 Jan 2006")
}

// GetIcon returns icon for notification type
func (n *Notification) GetIcon() string {
	switch NotificationType(n.Type) {
	case NotifRequestCreated, NotifRequestApproved, NotifRequestRejected, NotifRequestCompleted:
		return "bell"
	case NotifItemAdded, NotifItemUpdated:
		return "package"
	case NotifItemOutOfStock, NotifItemAvailable:
		return "alert-circle"
	case NotifQuotaReset, NotifQuotaExceeded:
		return "user"
	case NotifReminderReturn, NotifOverdueItem:
		return "clock"
	case NotifWelcome:
		return "heart"
	case NotifSystemUpdate:
		return "settings"
	case NotifPromotion:
		return "gift"
	default:
		return "bell"
	}
}

// GetColor returns color for notification type
func (n *Notification) GetColor() string {
	switch NotificationType(n.Type) {
	case NotifRequestApproved, NotifRequestCompleted, NotifItemAvailable, NotifQuotaReset:
		return "green"
	case NotifRequestRejected, NotifItemOutOfStock, NotifQuotaExceeded, NotifOverdueItem:
		return "red"
	case NotifReminderReturn:
		return "yellow"
	case NotifWelcome, NotifPromotion:
		return "blue"
	default:
		return "gray"
	}
}

// GetPriority returns priority level for notification
func (n *Notification) GetPriority() string {
	switch NotificationType(n.Type) {
	case NotifOverdueItem, NotifQuotaExceeded:
		return "high"
	case NotifRequestApproved, NotifRequestRejected, NotifReminderReturn:
		return "medium"
	default:
		return "low"
	}
}