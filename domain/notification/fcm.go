package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/database"
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/google/uuid"
)

// FCMService handles Firebase Cloud Messaging operations
type FCMService struct {
	client *messaging.Client
	config *config.Config
	db     *database.Database
}

// NewFCMService creates a new FCM service with Firebase Admin SDK
func NewFCMService(config *config.Config, db *database.Database) (*FCMService, error) {
	credentials := map[string]interface{}{
		"type":                        "service_account",
		"project_id":                  config.FirebaseProjectID,
		"private_key":                 config.FirebasePrivateKey,
		"client_email":                config.FirebaseClientEmail,
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	}

	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Firebase credentials: %v", err)
	}

	opt := option.WithCredentialsJSON(credentialsJSON)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %v", err)
	}

	client, err := app.Messaging(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get messaging client: %v", err)
	}

	return &FCMService{
		client: client,
		config: config,
		db:     db,
	}, nil
}

// SendNotification sends a push notification to a specific token
func (f *FCMService) SendNotification(ctx context.Context, token string, notification *models.Notification) error {
	if !f.config.EnablePushNotifications {
		return nil
	}

	data := make(map[string]string)
	data["type"] = notification.Type

	if notification.RelatedID != nil {
		data["related_id"] = notification.RelatedID.String()
	}

	switch models.NotificationType(notification.Type) {
	case models.NotifRequestCreated, models.NotifRequestApproved, models.NotifRequestRejected, models.NotifRequestCompleted:
		data["screen"] = "RequestDetail"
		data["action"] = "VIEW_REQUEST"
		if notification.Data != nil {
			if requestID, ok := notification.Data["request_id"].(string); ok {
				data["request_id"] = requestID
			}
			if itemName, ok := notification.Data["item_name"].(string); ok {
				data["item_name"] = itemName
			}
		}
	case models.NotifSystemUpdate, models.NotifPromotion:
		data["screen"] = "NotificationDetail"
		data["action"] = "VIEW_NOTIFICATION"
	default:
		data["screen"] = "Notifications"
		data["action"] = "VIEW_NOTIFICATIONS"
	}

	message := &messaging.Message{
		Notification: &messaging.Notification{
			Title: notification.Title,
			Body:  notification.Message,
		},
		Data:  data,
		Token: token,
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "satu_lemari_notifications",
				Priority:  messaging.PriorityHigh,
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title: notification.Title,
						Body:  notification.Message,
					},
					Badge: func() *int { i := 1; return &i }(),
					Sound: "default",
				},
			},
		},
	}

	response, err := f.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send FCM message: %v", err)
	}

	log.Printf("FCM message sent successfully: %s", response)
	return nil
}

// SendNotificationToUser sends a push notification to all devices of a user
func (f *FCMService) SendNotificationToUser(ctx context.Context, userID string, notification *models.Notification) error {
	tokens, err := f.GetUserTokens(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user tokens: %v", err)
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for user %s", userID)
		return nil
	}

	var successfulTokens []string
	for _, token := range tokens {
		err := f.SendNotification(ctx, token, notification)
		if err != nil {
			log.Printf("Failed to send FCM to token %s: %v", token, err)
		} else {
			successfulTokens = append(successfulTokens, token)
		}
	}

	// Update last_used_at for successful tokens
	if len(successfulTokens) > 0 {
		f.UpdateLastUsed(ctx, successfulTokens)
	}

	return nil
}

// GetUserTokens gets FCM tokens for a user from database
func (f *FCMService) GetUserTokens(ctx context.Context, userID string) ([]string, error) {
	log.Printf("Getting FCM tokens for user: %s", userID)

	// Use Supabase REST API to query FCM tokens
	filters := map[string]string{
		"user_id":   "eq." + userID,
		"is_active": "eq.true",
		"select":    "token",
		"order":     "last_used_at.desc",
	}

	log.Printf("Query filters: %+v", filters)
	rows, err := f.db.QueryTable(ctx, "fcm_tokens", filters)
	if err != nil {
		log.Printf("Failed to query FCM tokens: %v", err)
		return nil, fmt.Errorf("failed to query FCM tokens: %v", err)
	}
	defer rows.Close()

	// Parse JSON response
	var tokenData []map[string]interface{}
	if err := rows.Scan(&tokenData); err != nil {
		log.Printf("Failed to scan FCM tokens: %v", err)
		return nil, fmt.Errorf("failed to scan FCM tokens: %v", err)
	}

	log.Printf("Found %d token records for user %s", len(tokenData), userID)

	var tokens []string
	for _, data := range tokenData {
		if token, ok := data["token"].(string); ok {
			tokens = append(tokens, token)
		}
	}

	log.Printf("Returning %d tokens for user %s", len(tokens), userID)
	return tokens, nil
}

// RegisterToken registers a new FCM token for a user
func (f *FCMService) RegisterToken(ctx context.Context, userID string, token string, platform string) error {
	log.Printf("Registering FCM token for user %s, platform %s", userID, platform)

	// Manual upsert: Try update first, then insert if no rows affected
	// Step 1: Try to update existing record
	updateFilters := map[string]string{
		"user_id":  "eq." + userID,
		"platform": "eq." + platform,
	}

	updateData := map[string]interface{}{
		"token":        token,
		"is_active":    true,
		"updated_at":   time.Now().Format(time.RFC3339),
		"last_used_at": time.Now().Format(time.RFC3339),
	}

	rows, err := f.db.Update(ctx, "fcm_tokens", updateFilters, updateData)
	if err != nil {
		log.Printf("Update failed: %v", err)
		// Continue to insert
	} else {
		// Check if any rows were updated
		var result []map[string]interface{}
		if err := rows.Scan(&result); err == nil && len(result) > 0 {
			log.Printf("Successfully updated existing FCM token for user %s on platform %s", userID, platform)
			return nil
		}
		log.Printf("No existing token found, will insert new one")
	}

	// Step 2: Insert new record if update didn't affect any rows
	insertData := map[string]interface{}{
		"id":           uuid.New().String(),
		"user_id":      userID,
		"token":        token,
		"platform":     platform,
		"is_active":    true,
		"created_at":   time.Now().Format(time.RFC3339),
		"updated_at":   time.Now().Format(time.RFC3339),
		"last_used_at": time.Now().Format(time.RFC3339),
	}

	_, err = f.db.Insert(ctx, "fcm_tokens", insertData)
	if err != nil {
		log.Printf("Insert failed: %v", err)
		return fmt.Errorf("failed to register FCM token: %v", err)
	}

	log.Printf("Successfully inserted new FCM token for user %s on platform %s", userID, platform)
	return nil
}

// UpdateToken updates an existing FCM token
func (f *FCMService) UpdateToken(ctx context.Context, userID string, oldToken string, newToken string, platform string) error {
	log.Printf("Updating FCM token for user %s: %s -> %s", userID, oldToken, newToken)

	// Simply register the new token - upsert will handle replacement
	return f.RegisterToken(ctx, userID, newToken, platform)
}

// RemoveToken removes an FCM token (soft delete)
func (f *FCMService) RemoveToken(ctx context.Context, userID string, token string) error {
	log.Printf("Removing FCM token for user %s, token: %s", userID, token)

	filters := map[string]string{
		"token":   "eq." + token,
		"user_id": "eq." + userID,
	}

	updateData := map[string]interface{}{
		"is_active":  false,
		"updated_at": time.Now().Format(time.RFC3339),
	}

	log.Printf("RemoveToken filters: %+v", filters)
	rows, err := f.db.Update(ctx, "fcm_tokens", filters, updateData)
	if err != nil {
		log.Printf("Failed to remove FCM token: %v", err)
		return fmt.Errorf("failed to remove FCM token: %v", err)
	}

	// Check if any rows were updated
	var result []map[string]interface{}
	if err := rows.Scan(&result); err == nil {
		log.Printf("RemoveToken result: %d rows affected", len(result))
		if len(result) == 0 {
			log.Printf("Warning: No FCM token found to remove for user %s with token %s", userID, token)
		}
	}

	log.Printf("Successfully removed FCM token for user %s", userID)
	return nil
}

// UpdateLastUsed updates the last_used_at timestamp for FCM tokens
func (f *FCMService) UpdateLastUsed(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	// Update each token individually (Supabase REST API limitation)
	for _, token := range tokens {
		filters := map[string]string{
			"token":     "eq." + token,
			"is_active": "eq.true",
		}

		updateData := map[string]interface{}{
			"last_used_at": time.Now().Format(time.RFC3339),
		}

		_, err := f.db.Update(ctx, "fcm_tokens", filters, updateData)
		if err != nil {
			log.Printf("Failed to update last_used_at for FCM token %s: %v", token, err)
			// Continue with other tokens
		}
	}

	return nil
}

// SendToTopic sends a notification to a topic
func (f *FCMService) SendToTopic(ctx context.Context, topic string, title string, body string, data map[string]string) error {
	message := &messaging.Message{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data:  data,
		Topic: topic,
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "satu_lemari_notifications",
				Priority:  messaging.PriorityHigh,
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title: title,
						Body:  body,
					},
					Badge: func() *int { i := 1; return &i }(),
					Sound: "default",
				},
			},
		},
	}

	response, err := f.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send FCM topic message: %v", err)
	}

	log.Printf("FCM topic message sent successfully: %s", response)
	return nil
}

// SubscribeToTopic subscribes tokens to a topic
func (f *FCMService) SubscribeToTopic(ctx context.Context, tokens []string, topic string) error {
	response, err := f.client.SubscribeToTopic(ctx, tokens, topic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %v", err)
	}

	log.Printf("Subscribed %d tokens to topic %s. Success: %d, Failure: %d",
		len(tokens), topic, response.SuccessCount, response.FailureCount)

	return nil
}

// UnsubscribeFromTopic unsubscribes tokens from a topic
func (f *FCMService) UnsubscribeFromTopic(ctx context.Context, tokens []string, topic string) error {
	response, err := f.client.UnsubscribeFromTopic(ctx, tokens, topic)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe from topic: %v", err)
	}

	log.Printf("Unsubscribed %d tokens from topic %s. Success: %d, Failure: %d",
		len(tokens), topic, response.SuccessCount, response.FailureCount)

	return nil
}
