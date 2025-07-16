package notification

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/database"
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/google/uuid"
)

// NotificationService handles all notification operations
type NotificationService struct {
	config *config.Config
	cache  *cache.RedisCache
	email  *EmailService
	fcm    *FCMService
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

func NewNotificationService(cfg *config.Config, cache *cache.RedisCache, db *database.Database) (*NotificationService, error) {
	emailService := NewEmailService(cfg)
	fcmService, err := NewFCMService(cfg, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create FCM service: %v", err)
	}

	return &NotificationService{
		config: cfg,
		cache:  cache,
		email:  emailService,
		fcm:    fcmService,
	}, nil
}

func (n *NotificationService) SendNotification(ctx context.Context, userID uuid.UUID, template *NotificationTemplate) error {
	log.Printf("SendNotification called for user %s with template: %s", userID.String(), template.ID)

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

	log.Printf("Created notification object: ID=%s, UserID=%s, Title=%s",
		notification.ID.String(), notification.UserID.String(), notification.Title)

	// Store notification in database (primary storage)
	err := n.storeNotificationInDB(ctx, notification)
	if err != nil {
		log.Printf("Failed to store notification in database: %v", err)
		return fmt.Errorf("failed to store notification: %v", err)
	}

	// Also store in cache for faster access (optional)
	err = n.storeNotification(ctx, notification)
	if err != nil {
		log.Printf("Failed to store notification in cache: %v", err)
		// Don't return error, cache is optional
	}

	for _, channel := range template.Channels {
		switch channel {
		case "web":
			err = n.storeInAppNotification(ctx, notification)
			if err != nil {
				log.Printf("Failed to store in-app notification: %v", err)
			}
		case "mobile":
			err = n.storeInAppNotification(ctx, notification)
			if err != nil {
				log.Printf("Failed to store in-app notification: %v", err)
			}

			if n.config.EnablePushNotifications {
				err = n.sendFCMNotification(ctx, userID.String(), notification)
				if err != nil {
					log.Printf("Failed to send FCM notification: %v", err)
				} else {
					notification.IsSent = true
					now := time.Now()
					notification.SentAt = &now
				}
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

	err = n.storeNotification(ctx, notification)
	if err != nil {
		log.Printf("Failed to update notification status: %v", err)
	}

	return nil
}

func (n *NotificationService) SendBulkNotification(ctx context.Context, userIDs []uuid.UUID, template *NotificationTemplate) error {
	for _, userID := range userIDs {
		err := n.SendNotification(ctx, userID, template)
		if err != nil {
			log.Printf("Failed to send notification to user %s: %v", userID, err)
		}
	}
	return nil
}

func (n *NotificationService) GetUserNotifications(ctx context.Context, userID string, filters *models.NotificationFilter, pagination *common.PaginationParams) ([]*models.Notification, int, error) {
	log.Printf("Getting notifications for user %s with filters: %+v", userID, filters)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for notifications")
		return []*models.Notification{}, 0, nil
	}

	// Build query filters
	queryFilters := map[string]string{
		"user_id": "eq." + userID,
		"order":   "created_at.desc",
	}

	// Add pagination
	if pagination != nil {
		queryFilters["limit"] = fmt.Sprintf("%d", pagination.Limit)
		queryFilters["offset"] = fmt.Sprintf("%d", (pagination.Page-1)*pagination.Limit)
	}

	// Add filters
	if filters != nil && filters.IsRead != nil {
		queryFilters["is_read"] = fmt.Sprintf("eq.%t", *filters.IsRead)
	}

	log.Printf("Query filters: %+v", queryFilters)

	// Query notifications from database
	rows, err := n.fcm.db.QueryTable(ctx, "notifications", queryFilters)
	if err != nil {
		log.Printf("Failed to query notifications: %v", err)
		return nil, 0, fmt.Errorf("failed to query notifications: %v", err)
	}

	var dbNotifications []map[string]interface{}
	if err := rows.Scan(&dbNotifications); err != nil {
		log.Printf("Failed to scan notifications: %v", err)
		return nil, 0, fmt.Errorf("failed to scan notifications: %v", err)
	}

	log.Printf("Found %d notifications in database", len(dbNotifications))

	// Convert to models.Notification
	notifications := make([]*models.Notification, 0, len(dbNotifications))
	for _, dbNotif := range dbNotifications {
		// Parse ID
		idStr, _ := dbNotif["id"].(string)
		notifID, err := uuid.Parse(idStr)
		if err != nil {
			log.Printf("Invalid notification ID: %s", idStr)
			continue
		}

		// Get UserID (Firebase UID as string)
		userIDStr, _ := dbNotif["user_id"].(string)

		// Parse CreatedAt
		createdAtStr, _ := dbNotif["created_at"].(string)
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			log.Printf("Invalid created_at: %s", createdAtStr)
			createdAt = time.Now()
		}

		notification := &models.Notification{
			ID:        notifID,
			UserID:    userIDStr,
			Title:     dbNotif["title"].(string),
			Message:   dbNotif["message"].(string),
			Type:      dbNotif["type"].(string),
			IsRead:    dbNotif["is_read"].(bool),
			CreatedAt: createdAt,
		}

		// Handle data field (JSON)
		if dataField, ok := dbNotif["data"]; ok && dataField != nil {
			if dataMap, ok := dataField.(map[string]interface{}); ok {
				notification.Data = dataMap
			}
		}

		notifications = append(notifications, notification)
	}

	// Get total count (simplified for now)
	total := len(notifications)
	if pagination != nil && len(notifications) == pagination.Limit {
		// If we got full page, there might be more
		total = pagination.Page * pagination.Limit
	}

	log.Printf("Returning %d notifications for user %s", len(notifications), userID)
	return notifications, total, nil
}

func (n *NotificationService) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	key := n.generateUserUnreadKey(userID)

	var count int
	err := n.cache.Get(ctx, key, &count)
	if err != nil {
		return 0, nil
	}

	return count, nil
}

func (n *NotificationService) MarkAsRead(ctx context.Context, userID string, notificationID uuid.UUID) error {
	log.Printf("Marking notification %s as read for user %s", notificationID.String(), userID)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for marking notification as read")
		return fmt.Errorf("database not available")
	}

	// Update notification in database
	filters := map[string]string{
		"id":      "eq." + notificationID.String(),
		"user_id": "eq." + userID,
	}

	updateData := map[string]interface{}{
		"is_read": true,
		"read_at": time.Now().Format(time.RFC3339),
	}

	log.Printf("Updating notification with filters: %+v, data: %+v", filters, updateData)

	rows, err := n.fcm.db.Update(ctx, "notifications", filters, updateData)
	if err != nil {
		log.Printf("Failed to update notification: %v", err)
		return fmt.Errorf("failed to mark notification as read: %v", err)
	}

	// Check if any rows were updated
	var result []map[string]interface{}
	if err := rows.Scan(&result); err == nil {
		log.Printf("MarkAsRead result: %d rows affected", len(result))
		if len(result) == 0 {
			log.Printf("Warning: No notification found to mark as read for user %s with ID %s", userID, notificationID.String())
			return fmt.Errorf("notification not found or already read")
		}
	}

	log.Printf("Successfully marked notification %s as read for user %s", notificationID.String(), userID)
	return nil
}

func (n *NotificationService) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	log.Printf("Marking all notifications as read for user %s", userID)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for marking all notifications as read")
		return 0, fmt.Errorf("database not available")
	}

	// Update all unread notifications for user
	filters := map[string]string{
		"user_id": "eq." + userID,
		"is_read": "eq.false",
	}

	updateData := map[string]interface{}{
		"is_read": true,
		"read_at": time.Now().Format(time.RFC3339),
	}

	log.Printf("Updating all notifications with filters: %+v, data: %+v", filters, updateData)

	rows, err := n.fcm.db.Update(ctx, "notifications", filters, updateData)
	if err != nil {
		log.Printf("Failed to update all notifications: %v", err)
		return 0, fmt.Errorf("failed to mark all notifications as read: %v", err)
	}

	// Check how many rows were updated
	var result []map[string]interface{}
	updatedCount := 0
	if err := rows.Scan(&result); err == nil {
		updatedCount = len(result)
		log.Printf("MarkAllAsRead result: %d rows affected", updatedCount)
	}

	log.Printf("Successfully marked %d notifications as read for user %s", updatedCount, userID)
	return updatedCount, nil
}

// MarkMultipleAsRead marks multiple notifications as read
func (n *NotificationService) MarkMultipleAsRead(ctx context.Context, userID string, notificationIDs []uuid.UUID) (int, error) {
	log.Printf("Marking %d notifications as read for user %s", len(notificationIDs), userID)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for marking notifications as read")
		return 0, fmt.Errorf("database not available")
	}

	if len(notificationIDs) == 0 {
		return 0, nil
	}

	updatedCount := 0
	updateData := map[string]interface{}{
		"is_read": true,
		"read_at": time.Now().Format(time.RFC3339),
	}

	// Update each notification individually
	for _, notificationID := range notificationIDs {
		filters := map[string]string{
			"id":      "eq." + notificationID.String(),
			"user_id": "eq." + userID,
		}

		log.Printf("Updating notification %s with filters: %+v", notificationID.String(), filters)

		rows, err := n.fcm.db.Update(ctx, "notifications", filters, updateData)
		if err != nil {
			log.Printf("Failed to update notification %s: %v", notificationID.String(), err)
			continue // Continue with other notifications
		}

		// Check if this notification was updated
		var result []map[string]interface{}
		if err := rows.Scan(&result); err == nil && len(result) > 0 {
			updatedCount++
			log.Printf("Successfully marked notification %s as read", notificationID.String())
		} else {
			log.Printf("Warning: Notification %s not found or already read", notificationID.String())
		}
	}

	log.Printf("Successfully marked %d out of %d notifications as read for user %s", updatedCount, len(notificationIDs), userID)
	return updatedCount, nil
}

func (n *NotificationService) DeleteNotification(ctx context.Context, userID string, notificationID uuid.UUID) error {
	log.Printf("Deleting notification %s for user %s", notificationID.String(), userID)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for deleting notification")
		return fmt.Errorf("database not available")
	}

	// Delete notification from database
	filters := map[string]string{
		"id":      "eq." + notificationID.String(),
		"user_id": "eq." + userID,
	}

	log.Printf("Deleting notification with filters: %+v", filters)

	err := n.fcm.db.Delete(ctx, "notifications", filters)
	if err != nil {
		log.Printf("Failed to delete notification: %v", err)
		return fmt.Errorf("failed to delete notification: %v", err)
	}

	log.Printf("Successfully deleted notification %s for user %s", notificationID.String(), userID)
	return nil
}

// GetNotificationTemplates returns predefined notification templates
func (n *NotificationService) GetNotificationTemplates() map[string]*NotificationTemplate {
	return GetAllTemplates()
}

// SendRequestNotification sends notification for request status changes
func (n *NotificationService) SendRequestNotification(ctx context.Context, userID uuid.UUID, requestType, status, itemName string, requestID uuid.UUID) error {
	log.Printf("SendRequestNotification called: userID=%s, requestType=%s, status=%s, itemName=%s, requestID=%s",
		userID.String(), requestType, status, itemName, requestID.String())

	var templateID string
	switch status {
	case "pending", "created":
		templateID = "request_created"
	case "new_request":
		templateID = "new_request"
	case "approved":
		templateID = "request_approved"
	case "rejected":
		templateID = "request_rejected"
	case "completed":
		templateID = "request_completed"
	default:
		log.Printf("Unknown request status: %s", status)
		return fmt.Errorf("unknown request status: %s", status)
	}

	log.Printf("Using template: %s for status: %s", templateID, status)

	// Create notification with placeholder substitution
	template, err := CreateRequestNotification(templateID, requestType, itemName, requestID.String())
	if err != nil {
		log.Printf("Failed to create request notification template: %v", err)
		return fmt.Errorf("failed to create request notification: %v", err)
	}

	log.Printf("Created notification template: %+v", template)

	err = n.SendNotification(ctx, userID, template)
	if err != nil {
		log.Printf("Failed to send notification: %v", err)
		return err
	}

	log.Printf("Successfully sent notification to user %s", userID.String())
	return nil
}

// SendRequestNotificationWithFirebaseUID sends notification using Firebase UID (string) instead of UUID
func (n *NotificationService) SendRequestNotificationWithFirebaseUID(ctx context.Context, firebaseUID, requestType, status, itemName string, requestID uuid.UUID) error {
	log.Printf("SendRequestNotificationWithFirebaseUID called: firebaseUID=%s, requestType=%s, status=%s, itemName=%s, requestID=%s",
		firebaseUID, requestType, status, itemName, requestID.String())

	var templateID string
	switch status {
	case "pending", "created":
		templateID = "request_created"
	case "new_request":
		templateID = "new_request"
	case "approved":
		templateID = "request_approved"
	case "rejected":
		templateID = "request_rejected"
	case "completed":
		templateID = "request_completed"
	default:
		log.Printf("Unknown request status: %s", status)
		return fmt.Errorf("unknown request status: %s", status)
	}

	log.Printf("Using template: %s for status: %s", templateID, status)

	// Create notification with placeholder substitution
	template, err := CreateRequestNotification(templateID, requestType, itemName, requestID.String())
	if err != nil {
		log.Printf("Failed to create request notification template: %v", err)
		return fmt.Errorf("failed to create request notification: %v", err)
	}

	log.Printf("Created notification template: %+v", template)

	// Use Firebase UID directly for notification
	err = n.SendNotificationWithFirebaseUID(ctx, firebaseUID, template)
	if err != nil {
		log.Printf("Failed to send notification: %v", err)
		return err
	}

	log.Printf("Successfully sent notification to Firebase user %s", firebaseUID)
	return nil
}

// SendNotificationWithFirebaseUID sends notification using Firebase UID as string
func (n *NotificationService) SendNotificationWithFirebaseUID(ctx context.Context, firebaseUID string, template *NotificationTemplate) error {
	log.Printf("SendNotificationWithFirebaseUID called for Firebase user %s with template: %s", firebaseUID, template.ID)

	// Create notification with Firebase UID as string
	notification := &Notification{
		ID:        uuid.New(),
		UserID:    uuid.New(), // Generate UUID for notification ID, but store Firebase UID separately
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

	log.Printf("Created notification object: ID=%s, FirebaseUID=%s, Title=%s",
		notification.ID.String(), firebaseUID, notification.Title)

	// Store notification in database (primary storage) with Firebase UID
	err := n.storeNotificationInDBWithFirebaseUID(ctx, notification, firebaseUID)
	if err != nil {
		log.Printf("Failed to store notification in database: %v", err)
		return fmt.Errorf("failed to store notification: %v", err)
	}

	// Also store in cache for faster access (optional)
	err = n.storeNotification(ctx, notification)
	if err != nil {
		log.Printf("Failed to store notification in cache: %v", err)
		// Don't return error, cache is optional
	}

	// Send FCM notification if user has tokens
	if n.fcm != nil {
		go func() {
			// Use background context to avoid cancellation when HTTP request ends
			bgCtx := context.Background()
			tokens, err := n.fcm.GetUserTokens(bgCtx, firebaseUID)
			if err != nil {
				log.Printf("Failed to get FCM tokens for user %s: %v", firebaseUID, err)
				return
			}

			if len(tokens) > 0 {
				// Convert internal Notification to models.Notification for FCM
				fcmNotification := &models.Notification{
					ID:        notification.ID,
					UserID:    firebaseUID,
					Title:     notification.Title,
					Message:   notification.Message,
					Type:      notification.Type,
					Data:      notification.Data,
					IsRead:    notification.IsRead,
					CreatedAt: notification.CreatedAt,
				}

				for _, token := range tokens {
					err = n.fcm.SendNotification(bgCtx, token, fcmNotification)
					if err != nil {
						log.Printf("Failed to send FCM notification to token %s: %v", token, err)
					} else {
						log.Printf("FCM notification sent to token %s for user %s", token, firebaseUID)
					}
				}
			} else {
				log.Printf("No FCM tokens found for user %s", firebaseUID)
			}
		}()
	}

	return nil
}

// storeNotificationInDBWithFirebaseUID stores notification in database using Firebase UID
func (n *NotificationService) storeNotificationInDBWithFirebaseUID(ctx context.Context, notification *Notification, firebaseUID string) error {
	log.Printf("storeNotificationInDBWithFirebaseUID called for notification ID: %s, Firebase UID: %s",
		notification.ID.String(), firebaseUID)

	// Prepare notification data for database with Firebase UID
	notificationData := map[string]interface{}{
		"id":         notification.ID.String(),
		"user_id":    firebaseUID, // Use Firebase UID directly
		"title":      notification.Title,
		"message":    notification.Message,
		"type":       notification.Type,
		"data":       notification.Data,
		"is_read":    notification.IsRead,
		"created_at": notification.CreatedAt.Format(time.RFC3339),
	}

	log.Printf("Notification data to insert: %+v", notificationData)

	// Add database service reference
	if n.fcm != nil && n.fcm.db != nil {
		log.Printf("Database available, inserting notification...")
		_, err := n.fcm.db.Insert(ctx, "notifications", notificationData)
		if err != nil {
			log.Printf("Failed to insert notification into database: %v", err)
			return fmt.Errorf("failed to insert notification into database: %v", err)
		}
		log.Printf("Notification stored in database successfully: %s for Firebase user %s",
			notification.ID.String(), firebaseUID)
	} else {
		log.Printf("Warning: Database not available (fcm=%v, db=%v), notification not persisted",
			n.fcm != nil, n.fcm != nil && n.fcm.db != nil)
	}

	return nil
}

// SendTransactionNotification sends notification for transaction updates
func (n *NotificationService) SendTransactionNotification(ctx context.Context, userID uuid.UUID, transactionType, itemName string, transactionID uuid.UUID) error {
	var templateID string
	switch transactionType {
	case "received":
		templateID = "item_received"
	case "returned":
		templateID = "item_returned"
	default:
		return fmt.Errorf("unknown transaction type: %s", transactionType)
	}

	// Create notification with placeholder substitution
	template, err := CreateItemNotification(templateID, itemName, transactionID.String(), map[string]interface{}{
		"transaction_id": transactionID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to create transaction notification: %v", err)
	}

	return n.SendNotification(ctx, userID, template)
}

// SendQuotaResetNotification sends notification for quota reset
func (n *NotificationService) SendQuotaResetNotification(ctx context.Context, userID uuid.UUID) error {
	template, exists := GetTemplate("quota_reset")
	if !exists {
		return fmt.Errorf("quota reset template not found")
	}

	return n.SendNotification(ctx, userID, template)
}

// SendReminderNotification sends reminder notifications
func (n *NotificationService) SendReminderNotification(ctx context.Context, userID uuid.UUID, reminderType, itemName, dueDate string, relatedID uuid.UUID) error {
	var templateID string
	switch reminderType {
	case "pickup":
		templateID = "reminder_pickup"
	case "return":
		templateID = "reminder_return"
	default:
		return fmt.Errorf("unknown reminder type: %s", reminderType)
	}

	// Create notification with placeholder substitution
	template, err := CreateItemNotification(templateID, itemName, relatedID.String(), map[string]interface{}{
		"due_date":   dueDate,
		"related_id": relatedID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to create reminder notification: %v", err)
	}

	return n.SendNotification(ctx, userID, template)
}

// Helper functions
// storeNotificationInDB stores notification in database (primary storage)
func (n *NotificationService) storeNotificationInDB(ctx context.Context, notification *Notification) error {
	log.Printf("storeNotificationInDB called for notification ID: %s, user: %s",
		notification.ID.String(), notification.UserID.String())

	// Prepare notification data for database
	notificationData := map[string]interface{}{
		"id":         notification.ID.String(),
		"user_id":    notification.UserID.String(),
		"title":      notification.Title,
		"message":    notification.Message,
		"type":       notification.Type,
		"data":       notification.Data,
		"is_read":    notification.IsRead,
		"created_at": notification.CreatedAt.Format(time.RFC3339),
	}

	log.Printf("Notification data to insert: %+v", notificationData)

	// Add database service reference
	if n.fcm != nil && n.fcm.db != nil {
		log.Printf("Database available, inserting notification...")
		_, err := n.fcm.db.Insert(ctx, "notifications", notificationData)
		if err != nil {
			log.Printf("Failed to insert notification into database: %v", err)
			return fmt.Errorf("failed to insert notification into database: %v", err)
		}
		log.Printf("Notification stored in database successfully: %s for user %s",
			notification.ID.String(), notification.UserID.String())
	} else {
		log.Printf("Warning: Database not available (fcm=%v, db=%v), notification not persisted",
			n.fcm != nil, n.fcm != nil && n.fcm.db != nil)
	}

	return nil
}

func (n *NotificationService) storeNotification(ctx context.Context, notification *Notification) error {
	if n.cache == nil {
		log.Printf("Warning: Cache not available, skipping cache storage")
		return nil
	}
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

// sendFCMNotification sends FCM notification to user's devices
func (n *NotificationService) sendFCMNotification(ctx context.Context, userID string, notification *Notification) error {
	// Convert internal notification to models.Notification for FCM
	modelNotification := &models.Notification{
		ID:        notification.ID,
		UserID:    userID,
		Title:     notification.Title,
		Message:   notification.Message,
		Type:      notification.Type,
		RelatedID: notification.RelatedID,
		Data:      notification.Data,
		IsRead:    notification.IsRead,
		IsSent:    notification.IsSent,
		CreatedAt: notification.CreatedAt,
	}

	// Send to user's devices
	return n.fcm.SendNotificationToUser(ctx, userID, modelNotification)
}

// RegisterFCMToken registers a new FCM token for a user
func (n *NotificationService) RegisterFCMToken(ctx context.Context, userID string, token string, platform string) error {
	return n.fcm.RegisterToken(ctx, userID, token, platform)
}

// UpdateFCMToken updates an FCM token for a user
func (n *NotificationService) UpdateFCMToken(ctx context.Context, userID string, oldToken string, newToken string, platform string) error {
	return n.fcm.UpdateToken(ctx, userID, oldToken, newToken, platform)
}

// GetNotificationStats gets notification statistics for a user
func (n *NotificationService) GetNotificationStats(ctx context.Context, userID string) (*models.NotificationStats, error) {
	log.Printf("Getting notification stats for user %s", userID)

	if n.fcm == nil || n.fcm.db == nil {
		log.Printf("Database not available for notification stats")
		return &models.NotificationStats{}, fmt.Errorf("database not available")
	}

	// Initialize stats
	stats := &models.NotificationStats{
		TotalNotifications: 0,
		UnreadCount:        0,
		ReadCount:          0,
		TodayCount:         0,
		WeekCount:          0,
	}

	// 1. Get total notifications count
	totalFilters := map[string]string{
		"user_id": "eq." + userID,
	}

	log.Printf("Querying total notifications with filters: %+v", totalFilters)
	totalRows, err := n.fcm.db.QueryTable(ctx, "notifications", totalFilters)
	if err != nil {
		log.Printf("Failed to query total notifications: %v", err)
	} else {
		var totalResult []map[string]interface{}
		if err := totalRows.Scan(&totalResult); err == nil {
			stats.TotalNotifications = len(totalResult)
			log.Printf("Total notifications count: %d", stats.TotalNotifications)
		}
	}

	// 2. Get unread notifications count
	unreadFilters := map[string]string{
		"user_id": "eq." + userID,
		"is_read": "eq.false",
	}

	log.Printf("Querying unread notifications with filters: %+v", unreadFilters)
	unreadRows, err := n.fcm.db.QueryTable(ctx, "notifications", unreadFilters)
	if err != nil {
		log.Printf("Failed to query unread notifications: %v", err)
	} else {
		var unreadResult []map[string]interface{}
		if err := unreadRows.Scan(&unreadResult); err == nil {
			stats.UnreadCount = len(unreadResult)
			log.Printf("Unread notifications count: %d", stats.UnreadCount)
		}
	}

	// 3. Calculate read count
	stats.ReadCount = stats.TotalNotifications - stats.UnreadCount

	// 4. Get today's notifications count
	today := time.Now().Format("2006-01-02")
	todayFilters := map[string]string{
		"user_id":    "eq." + userID,
		"created_at": "gte." + today + "T00:00:00Z",
	}

	log.Printf("Querying today notifications with filters: %+v", todayFilters)
	todayRows, err := n.fcm.db.QueryTable(ctx, "notifications", todayFilters)
	if err != nil {
		log.Printf("Failed to query today notifications: %v", err)
	} else {
		var todayResult []map[string]interface{}
		if err := todayRows.Scan(&todayResult); err == nil {
			stats.TodayCount = len(todayResult)
			log.Printf("Today notifications count: %d", stats.TodayCount)
		}
	}

	// 5. Get this week's notifications count
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	weekFilters := map[string]string{
		"user_id":    "eq." + userID,
		"created_at": "gte." + weekAgo + "T00:00:00Z",
	}

	log.Printf("Querying week notifications with filters: %+v", weekFilters)
	weekRows, err := n.fcm.db.QueryTable(ctx, "notifications", weekFilters)
	if err != nil {
		log.Printf("Failed to query week notifications: %v", err)
	} else {
		var weekResult []map[string]interface{}
		if err := weekRows.Scan(&weekResult); err == nil {
			stats.WeekCount = len(weekResult)
			log.Printf("Week notifications count: %d", stats.WeekCount)
		}
	}

	log.Printf("Notification stats for user %s: Total=%d, Unread=%d, Read=%d, Today=%d, Week=%d",
		userID, stats.TotalNotifications, stats.UnreadCount, stats.ReadCount, stats.TodayCount, stats.WeekCount)

	return stats, nil
}

// CreateNotification creates a new notification (admin only)
func (n *NotificationService) CreateNotification(ctx context.Context, req *models.CreateNotificationRequest) (*models.Notification, error) {
	// Mock implementation - in real app, this would create in Supabase
	notification := &models.Notification{
		ID:        uuid.New(),
		UserID:    req.UserID,
		Title:     req.Title,
		Message:   req.Message,
		Type:      req.Type,
		Data:      req.Data,
		IsRead:    false,
		IsSent:    false,
		Platform:  req.Platform,
		CreatedAt: time.Now(),
	}

	// TODO: Implement actual database create
	log.Printf("Creating notification for user %s: %s", req.UserID, req.Title)

	return notification, nil
}

// BroadcastNotification sends a notification to multiple users (admin only)
func (n *NotificationService) BroadcastNotification(ctx context.Context, title, message, notifType string, data map[string]interface{}, platform string) (int, error) {
	// Mock implementation - in real app, this would broadcast to multiple users
	sentCount := 0

	// TODO: Implement actual broadcast logic
	log.Printf("Broadcasting notification to all users: %s", title)

	return sentCount, nil
}
