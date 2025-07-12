package notification

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/google/uuid"
)

// NotificationService handles all notification operations
type NotificationService struct {
	config *config.Config
	cache  *cache.RedisCache
	email  *EmailService
}

// Notification represents a notification
type Notification struct {
	ID        uuid.UUID              `json:"id"`
	UserID    uuid.UUID              `json:"user_id"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Type      string                 `json:"type"`
	RelatedID *uuid.UUID             `json:"related_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Channels  []string               `json:"channels"` // web, mobile, email
	Priority  string                 `json:"priority"` // low, normal, high, urgent
	IsRead    bool                   `json:"is_read"`
	IsSent    bool                   `json:"is_sent"`
	CreatedAt time.Time              `json:"created_at"`
	ReadAt    *time.Time             `json:"read_at,omitempty"`
	SentAt    *time.Time             `json:"sent_at,omitempty"`
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	ID       string                 `json:"id"`
	Title    string                 `json:"title"`
	Message  string                 `json:"message"`
	Type     string                 `json:"type"`
	Channels []string               `json:"channels"`
	Priority string                 `json:"priority"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// NewNotificationService creates a new notification service
func NewNotificationService(cfg *config.Config, cache *cache.RedisCache) *NotificationService {
	emailService := NewEmailService(cfg)

	return &NotificationService{
		config: cfg,
		cache:  cache,
		email:  emailService,
	}
}

// SendNotification sends a notification to a user
func (n *NotificationService) SendNotification(ctx context.Context, userID uuid.UUID, template *NotificationTemplate) error {
	notification := &Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     template.Title,
		Message:   template.Message,
		Type:      template.Type,
		Data:      template.Data,
		Channels:  template.Channels,
		Priority:  template.Priority,
		IsRead:    false,
		IsSent:    false,
		CreatedAt: time.Now(),
	}

	// Store notification in cache
	err := n.storeNotification(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to store notification: %v", err)
	}

	// Send through different channels
	for _, channel := range template.Channels {
		switch channel {
		case "web", "mobile":
			// Store for in-app notification
			err = n.storeInAppNotification(ctx, notification)
			if err != nil {
				log.Printf("Failed to store in-app notification: %v", err)
			}
		case "email":
			if n.config.EnableEmailNotifications {
				err = n.email.SendEmail(ctx, userID, notification)
				if err != nil {
					log.Printf("Failed to send email notification: %v", err)
				} else {
					notification.IsSent = true
					now := time.Now()
					notification.SentAt = &now
				}
			}

		}
	}

	// Update notification status
	err = n.storeNotification(ctx, notification)
	if err != nil {
		log.Printf("Failed to update notification status: %v", err)
	}

	return nil
}

// SendBulkNotification sends notifications to multiple users
func (n *NotificationService) SendBulkNotification(ctx context.Context, userIDs []uuid.UUID, template *NotificationTemplate) error {
	for _, userID := range userIDs {
		err := n.SendNotification(ctx, userID, template)
		if err != nil {
			log.Printf("Failed to send notification to user %s: %v", userID, err)
		}
	}
	return nil
}

// GetUserNotifications gets notifications for a user
func (n *NotificationService) GetUserNotifications(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Notification, error) {
	key := n.generateUserNotificationsKey(userID)

	// Get notifications from cache
	var notifications []*Notification
	err := n.cache.Get(ctx, key, &notifications)
	if err != nil {
		// If not in cache, return empty list
		return []*Notification{}, nil
	}

	// Apply pagination
	start := offset
	end := offset + limit
	if start >= len(notifications) {
		return []*Notification{}, nil
	}
	if end > len(notifications) {
		end = len(notifications)
	}

	return notifications[start:end], nil
}

// GetUnreadCount gets the count of unread notifications for a user
func (n *NotificationService) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	key := n.generateUserUnreadKey(userID)

	var count int
	err := n.cache.Get(ctx, key, &count)
	if err != nil {
		return 0, nil
	}

	return count, nil
}

// MarkAsRead marks a notification as read
func (n *NotificationService) MarkAsRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	// Get notification
	notificationKey := n.generateNotificationKey(notificationID)
	var notification Notification
	err := n.cache.Get(ctx, notificationKey, &notification)
	if err != nil {
		return fmt.Errorf("notification not found: %v", err)
	}

	// Check if notification belongs to user
	if notification.UserID != userID {
		return fmt.Errorf("notification does not belong to user")
	}

	// Mark as read
	notification.IsRead = true
	now := time.Now()
	notification.ReadAt = &now

	// Update notification
	err = n.storeNotification(ctx, &notification)
	if err != nil {
		return fmt.Errorf("failed to update notification: %v", err)
	}

	// Update unread count
	err = n.updateUnreadCount(ctx, userID, -1)
	if err != nil {
		log.Printf("Failed to update unread count: %v", err)
	}

	return nil
}

// MarkAllAsRead marks all notifications as read for a user
func (n *NotificationService) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	// Get all user notifications
	notifications, err := n.GetUserNotifications(ctx, userID, 1000, 0) // Get all
	if err != nil {
		return fmt.Errorf("failed to get user notifications: %v", err)
	}

	// Mark all as read
	now := time.Now()
	for _, notification := range notifications {
		if !notification.IsRead {
			notification.IsRead = true
			notification.ReadAt = &now
			err = n.storeNotification(ctx, notification)
			if err != nil {
				log.Printf("Failed to update notification %s: %v", notification.ID, err)
			}
		}
	}

	// Reset unread count
	err = n.cache.Set(ctx, n.generateUserUnreadKey(userID), 0, 24*time.Hour)
	if err != nil {
		log.Printf("Failed to reset unread count: %v", err)
	}

	return nil
}

// DeleteNotification deletes a notification
func (n *NotificationService) DeleteNotification(ctx context.Context, userID, notificationID uuid.UUID) error {
	// Get notification
	notificationKey := n.generateNotificationKey(notificationID)
	var notification Notification
	err := n.cache.Get(ctx, notificationKey, &notification)
	if err != nil {
		return fmt.Errorf("notification not found: %v", err)
	}

	// Check if notification belongs to user
	if notification.UserID != userID {
		return fmt.Errorf("notification does not belong to user")
	}

	// Delete notification
	err = n.cache.Delete(ctx, notificationKey)
	if err != nil {
		return fmt.Errorf("failed to delete notification: %v", err)
	}

	// Update unread count if notification was unread
	if !notification.IsRead {
		err = n.updateUnreadCount(ctx, userID, -1)
		if err != nil {
			log.Printf("Failed to update unread count: %v", err)
		}
	}

	return nil
}

// GetNotificationTemplates returns predefined notification templates
func (n *NotificationService) GetNotificationTemplates() map[string]*NotificationTemplate {
	return map[string]*NotificationTemplate{
		"request_created": {
			ID:       "request_created",
			Title:    "Permintaan Berhasil Dibuat",
			Message:  "Permintaan Anda telah berhasil dibuat dan sedang menunggu persetujuan.",
			Type:     "request_update",
			Channels: []string{"web", "mobile"},
			Priority: "normal",
		},
		"request_approved": {
			ID:       "request_approved",
			Title:    "Permintaan Diterima",
			Message:  "Permintaan Anda telah diterima. Silakan ambil barang sesuai petunjuk.",
			Type:     "request_update",
			Channels: []string{"web", "mobile", "email"},
			Priority: "high",
		},
		"request_rejected": {
			ID:       "request_rejected",
			Title:    "Permintaan Ditolak",
			Message:  "Permintaan Anda telah ditolak. Silakan cek detail untuk informasi lebih lanjut.",
			Type:     "request_update",
			Channels: []string{"web", "mobile", "email"},
			Priority: "normal",
		},
		"item_received": {
			ID:       "item_received",
			Title:    "Barang Diterima",
			Message:  "Barang telah berhasil diterima. Terima kasih telah menggunakan SatuLemari!",
			Type:     "transaction_update",
			Channels: []string{"web", "mobile"},
			Priority: "normal",
		},
		"item_returned": {
			ID:       "item_returned",
			Title:    "Barang Dikembalikan",
			Message:  "Barang telah berhasil dikembalikan. Terima kasih telah menggunakan SatuLemari!",
			Type:     "transaction_update",
			Channels: []string{"web", "mobile"},
			Priority: "normal",
		},
		"quota_reset": {
			ID:       "quota_reset",
			Title:    "Kuota Donasi Diperbarui",
			Message:  "Kuota donasi mingguan Anda telah diperbarui. Anda dapat mengajukan permintaan donasi baru.",
			Type:     "quota_update",
			Channels: []string{"web", "mobile"},
			Priority: "low",
		},
		"new_item_available": {
			ID:       "new_item_available",
			Title:    "Barang Baru Tersedia",
			Message:  "Ada barang baru yang sesuai dengan preferensi Anda. Segera cek sekarang!",
			Type:     "item_update",
			Channels: []string{"web", "mobile", "email"},
			Priority: "normal",
		},
		"reminder_pickup": {
			ID:       "reminder_pickup",
			Title:    "Pengingat Pengambilan",
			Message:  "Jangan lupa untuk mengambil barang yang telah disetujui dalam 24 jam ke depan.",
			Type:     "reminder",
			Channels: []string{"web", "mobile", "email"},
			Priority: "high",
		},
		"reminder_return": {
			ID:       "reminder_return",
			Title:    "Pengingat Pengembalian",
			Message:  "Jangan lupa untuk mengembalikan barang yang disewa sesuai tanggal yang ditentukan.",
			Type:     "reminder",
			Channels: []string{"web", "mobile", "email"},
			Priority: "high",
		},
	}
}

// SendRequestNotification sends notification for request status changes
func (n *NotificationService) SendRequestNotification(ctx context.Context, userID uuid.UUID, requestType, status string, requestID uuid.UUID) error {
	templates := n.GetNotificationTemplates()

	var template *NotificationTemplate
	switch status {
	case "pending":
		template = templates["request_created"]
	case "approved":
		template = templates["request_approved"]
	case "rejected":
		template = templates["request_rejected"]
	default:
		return fmt.Errorf("unknown request status: %s", status)
	}

	// Add request-specific data
	if template.Data == nil {
		template.Data = make(map[string]interface{})
	}
	template.Data["request_id"] = requestID
	template.Data["request_type"] = requestType

	return n.SendNotification(ctx, userID, template)
}

// SendTransactionNotification sends notification for transaction updates
func (n *NotificationService) SendTransactionNotification(ctx context.Context, userID uuid.UUID, transactionType string, transactionID uuid.UUID) error {
	templates := n.GetNotificationTemplates()

	var template *NotificationTemplate
	switch transactionType {
	case "received":
		template = templates["item_received"]
	case "returned":
		template = templates["item_returned"]
	default:
		return fmt.Errorf("unknown transaction type: %s", transactionType)
	}

	// Add transaction-specific data
	if template.Data == nil {
		template.Data = make(map[string]interface{})
	}
	template.Data["transaction_id"] = transactionID

	return n.SendNotification(ctx, userID, template)
}

// SendQuotaResetNotification sends notification for quota reset
func (n *NotificationService) SendQuotaResetNotification(ctx context.Context, userID uuid.UUID) error {
	templates := n.GetNotificationTemplates()
	template := templates["quota_reset"]

	return n.SendNotification(ctx, userID, template)
}

// SendReminderNotification sends reminder notifications
func (n *NotificationService) SendReminderNotification(ctx context.Context, userID uuid.UUID, reminderType string, relatedID uuid.UUID) error {
	templates := n.GetNotificationTemplates()

	var template *NotificationTemplate
	switch reminderType {
	case "pickup":
		template = templates["reminder_pickup"]
	case "return":
		template = templates["reminder_return"]
	default:
		return fmt.Errorf("unknown reminder type: %s", reminderType)
	}

	// Add reminder-specific data
	if template.Data == nil {
		template.Data = make(map[string]interface{})
	}
	template.Data["related_id"] = relatedID

	return n.SendNotification(ctx, userID, template)
}

// Helper functions
func (n *NotificationService) storeNotification(ctx context.Context, notification *Notification) error {
	key := n.generateNotificationKey(notification.ID)
	return n.cache.Set(ctx, key, notification, 7*24*time.Hour) // Store for 7 days
}

func (n *NotificationService) storeInAppNotification(ctx context.Context, notification *Notification) error {
	key := n.generateUserNotificationsKey(notification.UserID)

	// Get existing notifications
	var notifications []*Notification
	err := n.cache.Get(ctx, key, &notifications)
	if err != nil {
		notifications = []*Notification{}
	}

	// Add new notification at the beginning
	notifications = append([]*Notification{notification}, notifications...)

	// Keep only last 100 notifications
	if len(notifications) > 100 {
		notifications = notifications[:100]
	}

	// Store updated notifications
	err = n.cache.Set(ctx, key, notifications, 7*24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to store in-app notifications: %v", err)
	}

	// Update unread count
	if !notification.IsRead {
		err = n.updateUnreadCount(ctx, notification.UserID, 1)
		if err != nil {
			log.Printf("Failed to update unread count: %v", err)
		}
	}

	return nil
}

func (n *NotificationService) updateUnreadCount(ctx context.Context, userID uuid.UUID, delta int) error {
	key := n.generateUserUnreadKey(userID)

	var count int
	err := n.cache.Get(ctx, key, &count)
	if err != nil {
		count = 0
	}

	count += delta
	if count < 0 {
		count = 0
	}

	return n.cache.Set(ctx, key, count, 24*time.Hour)
}

func (n *NotificationService) generateNotificationKey(notificationID uuid.UUID) string {
	return fmt.Sprintf("notification:%s", notificationID.String())
}

func (n *NotificationService) generateUserNotificationsKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:notifications", userID.String())
}

func (n *NotificationService) generateUserUnreadKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:unread_count", userID.String())
}
