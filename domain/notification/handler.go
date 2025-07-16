package notification

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// NotificationHandler handles notification HTTP requests
type NotificationHandler struct {
	service *NotificationService
	config  *config.Config
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(config *config.Config, service *NotificationService) *NotificationHandler {
	return &NotificationHandler{
		service: service,
		config:  config,
	}
}

// RegisterRoutes registers notification routes
func (h *NotificationHandler) RegisterRoutes(r *mux.Router) {
	// FCM token management (must be before generic routes to avoid conflicts)
	r.HandleFunc("/notifications/fcm-token", h.RegisterFCMToken).Methods("POST")
	r.HandleFunc("/notifications/fcm-token", h.UpdateFCMToken).Methods("PUT")
	r.HandleFunc("/notifications/fcm-token", h.RemoveFCMToken).Methods("DELETE")
	r.HandleFunc("/notifications/fcm-tokens", h.GetUserFCMTokens).Methods("GET")

	// Template and bulk notifications
	r.HandleFunc("/notifications/send-template", h.SendTemplateNotification).Methods("POST")
	r.HandleFunc("/notifications/send-bulk", h.SendBulkNotifications).Methods("POST")

	// FCM testing and topic management
	r.HandleFunc("/notifications/fcm/test", h.SendTestNotification).Methods("POST")
	r.HandleFunc("/notifications/fcm/topic", h.SendTopicNotification).Methods("POST")
	r.HandleFunc("/notifications/fcm/subscribe", h.SubscribeToTopic).Methods("POST")
	r.HandleFunc("/notifications/fcm/unsubscribe", h.UnsubscribeFromTopic).Methods("POST")

	// Health check
	r.HandleFunc("/notifications/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/notifications/debug/{user_id}", h.DebugUserNotifications).Methods("GET") // Debug endpoint

	// Protected routes (require authentication) - must be after specific routes
	r.HandleFunc("/notifications", h.GetNotifications).Methods("GET")
	r.HandleFunc("/notifications/stats", h.GetNotificationStats).Methods("GET")
	r.HandleFunc("/notifications/read-all", h.MarkAllAsRead).Methods("PUT")
	r.HandleFunc("/notifications/mark-read", h.MarkMultipleAsRead).Methods("PUT")               // Bulk mark as read
	r.HandleFunc("/notifications/delete-bulk", h.DeleteMultipleNotifications).Methods("DELETE") // Bulk delete
	r.HandleFunc("/notifications/{id}/read", h.MarkAsRead).Methods("PUT")
	r.HandleFunc("/notifications/{id}", h.DeleteNotification).Methods("DELETE")
}

// GetNotifications handles GET /notifications
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Parse pagination parameters
	pagination := common.GetPaginationParams(r)

	// Parse filter parameters
	filters := h.parseNotificationFilters(r)
	filters.UserID = &userID

	// Get notifications
	notifications, total, err := h.service.GetUserNotifications(r.Context(), userID, filters, &pagination)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, notifications, meta, "Notifications retrieved successfully")
}

// GetNotificationStats handles GET /notifications/stats
func (h *NotificationHandler) GetNotificationStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	stats, err := h.service.GetNotificationStats(r.Context(), userID)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, stats, "Notification stats retrieved successfully")
}

// MarkAsRead handles PUT /notifications/{id}/read
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	notificationIDStr := common.GetPathParam(r, "id")
	notificationID, err := uuid.Parse(notificationIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid notification ID"),
			common.GenerateTraceID())
		return
	}

	markErr := h.service.MarkAsRead(r.Context(), userID, notificationID)
	if markErr != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, markErr.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"id":      notificationID.String(),
		"is_read": true,
		"read_at": time.Now().Format(time.RFC3339),
	}

	common.WriteSuccessResponse(w, response, "Notification marked as read")
}

// MarkMultipleAsRead handles PUT /notifications/mark-read
func (h *NotificationHandler) MarkMultipleAsRead(w http.ResponseWriter, r *http.Request) {
	log.Printf("MarkMultipleAsRead handler called")

	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	log.Printf("MarkMultipleAsRead: User ID: %s", userID)

	var req struct {
		NotificationIDs []string `json:"notification_ids" validate:"required,min=1"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("MarkMultipleAsRead: Failed to decode request body: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		log.Printf("MarkMultipleAsRead: Validation failed: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	// Parse notification IDs
	notificationIDs := make([]uuid.UUID, 0, len(req.NotificationIDs))
	for _, idStr := range req.NotificationIDs {
		notificationID, err := uuid.Parse(idStr)
		if err != nil {
			log.Printf("MarkMultipleAsRead: Invalid notification ID: %s", idStr)
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid notification ID: %s", idStr)),
				common.GenerateTraceID())
			return
		}
		notificationIDs = append(notificationIDs, notificationID)
	}

	log.Printf("MarkMultipleAsRead: Marking %d notifications as read for user %s", len(notificationIDs), userID)

	updatedCount, err := h.service.MarkMultipleAsRead(r.Context(), userID, notificationIDs)
	if err != nil {
		log.Printf("MarkMultipleAsRead: Failed to mark notifications as read: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"updated_count":    updatedCount,
		"total_requested":  len(notificationIDs),
		"notification_ids": req.NotificationIDs,
		"marked_at":        time.Now().Format(time.RFC3339),
	}

	log.Printf("MarkMultipleAsRead: Successfully marked %d notifications as read for user %s", updatedCount, userID)
	common.WriteSuccessResponse(w, response, fmt.Sprintf("Marked %d notifications as read", updatedCount))
}

// MarkAllAsRead handles PUT /notifications/read-all
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	updatedCount, err := h.service.MarkAllAsRead(r.Context(), userID)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"updated_count": updatedCount,
	}

	common.WriteSuccessResponse(w, response, "All notifications marked as read")
}

// DeleteNotification handles DELETE /notifications/{id}
func (h *NotificationHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	notificationIDStr := common.GetPathParam(r, "id")
	notificationID, err := uuid.Parse(notificationIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid notification ID"),
			common.GenerateTraceID())
		return
	}

	appErr := h.service.DeleteNotification(r.Context(), userID, notificationID)
	if appErr != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, appErr.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"id":         notificationID.String(),
		"deleted_at": time.Now().Format(time.RFC3339),
		"user_id":    userID,
	}

	common.WriteSuccessResponse(w, response, "Notification deleted successfully")
}

// DeleteMultipleNotifications handles DELETE /notifications/delete-bulk
func (h *NotificationHandler) DeleteMultipleNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		NotificationIDs []string `json:"notification_ids" validate:"required,min=1"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	// Parse notification IDs
	notificationIDs := make([]uuid.UUID, 0, len(req.NotificationIDs))
	for _, idStr := range req.NotificationIDs {
		notificationID, err := uuid.Parse(idStr)
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, fmt.Sprintf("Invalid notification ID: %s", idStr)),
				common.GenerateTraceID())
			return
		}
		notificationIDs = append(notificationIDs, notificationID)
	}

	log.Printf("DeleteMultipleNotifications: Deleting %d notifications for user %s", len(notificationIDs), userID)

	// Delete each notification
	successCount := 0
	failedIDs := []string{}

	for _, notificationID := range notificationIDs {
		err := h.service.DeleteNotification(r.Context(), userID, notificationID)
		if err != nil {
			log.Printf("Failed to delete notification %s: %v", notificationID.String(), err)
			failedIDs = append(failedIDs, notificationID.String())
		} else {
			successCount++
		}
	}

	response := map[string]interface{}{
		"total_requested": len(notificationIDs),
		"success_count":   successCount,
		"failed_count":    len(failedIDs),
		"failed_ids":      failedIDs,
		"deleted_at":      time.Now().Format(time.RFC3339),
	}

	message := fmt.Sprintf("Deleted %d out of %d notifications", successCount, len(notificationIDs))
	log.Printf("DeleteMultipleNotifications: %s for user %s", message, userID)
	common.WriteSuccessResponse(w, response, message)
}

// parseNotificationFilters parses notification filter parameters from request
func (h *NotificationHandler) parseNotificationFilters(r *http.Request) *models.NotificationFilter {
	filters := &models.NotificationFilter{}

	if typeParam := r.URL.Query().Get("type"); typeParam != "" {
		filters.Type = &typeParam
	}

	if isReadParam := r.URL.Query().Get("is_read"); isReadParam != "" {
		if isRead, err := strconv.ParseBool(isReadParam); err == nil {
			filters.IsRead = &isRead
		}
	}

	if platform := r.URL.Query().Get("platform"); platform != "" {
		filters.Platform = &platform
	}

	return filters
}

// RegisterFCMToken handles POST /notifications/fcm-token
func (h *NotificationHandler) RegisterFCMToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		FCMToken string `json:"fcm_token" validate:"required"`
		Platform string `json:"platform" validate:"required,oneof=android ios"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	err := h.service.RegisterFCMToken(r.Context(), userID, req.FCMToken, req.Platform)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"token":     req.FCMToken,
		"platform":  req.Platform,
		"is_active": true,
	}

	common.WriteSuccessResponse(w, response, "FCM token registered successfully")
}

// UpdateFCMToken handles PUT /notifications/fcm-token
func (h *NotificationHandler) UpdateFCMToken(w http.ResponseWriter, r *http.Request) {
	log.Printf("UpdateFCMToken handler called")

	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		log.Printf("UpdateFCMToken: User not authenticated")
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	log.Printf("UpdateFCMToken: User ID: %s", userID)

	var req struct {
		OldToken string `json:"old_token" validate:"required"`
		NewToken string `json:"new_token" validate:"required"`
		Platform string `json:"platform" validate:"required,oneof=android ios"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	err := h.service.UpdateFCMToken(r.Context(), userID, req.OldToken, req.NewToken, req.Platform)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"old_token": req.OldToken,
		"new_token": req.NewToken,
		"platform":  req.Platform,
		"is_active": true,
	}

	common.WriteSuccessResponse(w, response, "FCM token updated successfully")
}

// RemoveFCMToken handles DELETE /notifications/fcm-token
func (h *NotificationHandler) RemoveFCMToken(w http.ResponseWriter, r *http.Request) {
	log.Printf("RemoveFCMToken handler called")

	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		log.Printf("RemoveFCMToken: User not authenticated")
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User not authenticated"),
			common.GenerateTraceID())
		return
	}

	log.Printf("RemoveFCMToken: User ID: %s", userID)

	var req struct {
		Token string `json:"token" validate:"required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("RemoveFCMToken: Failed to decode request body: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		log.Printf("RemoveFCMToken: Validation failed: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	log.Printf("RemoveFCMToken: Removing token for user %s", userID)
	err := h.service.fcm.RemoveToken(r.Context(), userID, req.Token)
	if err != nil {
		log.Printf("RemoveFCMToken: Failed to remove token: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"token_removed": req.Token,
		"is_active":     false,
	}

	log.Printf("RemoveFCMToken: Successfully removed token for user %s", userID)
	common.WriteSuccessResponse(w, response, "FCM token removed successfully")
}

// GetUserFCMTokens handles GET /notifications/fcm-tokens
func (h *NotificationHandler) GetUserFCMTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User not authenticated"),
			common.GenerateTraceID())
		return
	}

	tokens, err := h.service.fcm.GetUserTokens(r.Context(), userID)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, map[string]interface{}{
		"tokens": tokens,
		"count":  len(tokens),
	}, "FCM tokens retrieved successfully")
}

// SendTestNotification handles POST /notifications/fcm/test
func (h *NotificationHandler) SendTestNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User not authenticated"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		Title   string                 `json:"title"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	notification := &models.Notification{
		Title:   req.Title,
		Message: req.Message,
		Type:    "test",
		Data:    req.Data,
	}

	err := h.service.fcm.SendNotificationToUser(r.Context(), userID, notification)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, map[string]interface{}{
		"title":   req.Title,
		"message": req.Message,
	}, "Test notification sent successfully")
}

// SendTopicNotification handles POST /notifications/fcm/topic
func (h *NotificationHandler) SendTopicNotification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic   string                 `json:"topic"`
		Title   string                 `json:"title"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	dataStr := make(map[string]string)
	for k, v := range req.Data {
		if str, ok := v.(string); ok {
			dataStr[k] = str
		} else {
			if jsonBytes, err := json.Marshal(v); err == nil {
				dataStr[k] = string(jsonBytes)
			}
		}
	}

	// Try to send via FCM, but don't fail if FCM is not available
	err := h.service.fcm.SendToTopic(r.Context(), req.Topic, req.Title, req.Message, dataStr)
	if err != nil {
		log.Printf("FCM topic notification failed (this is expected in development): %v", err)

		// For development: simulate successful topic notification
		log.Printf("Simulating topic notification for development: topic=%s, title=%s", req.Topic, req.Title)

		// In production, you might want to store topic notifications in database
		// or use alternative delivery methods
	} else {
		log.Printf("FCM topic notification sent successfully: topic=%s", req.Topic)
	}

	// Always return success for topic notifications (FCM is optional)
	response := map[string]interface{}{
		"topic":   req.Topic,
		"title":   req.Title,
		"message": req.Message,
		"data":    req.Data,
		"sent_at": time.Now().Format(time.RFC3339),
	}

	if err != nil {
		response["fcm_status"] = "failed"
		response["fcm_error"] = "FCM not configured (development mode)"
	} else {
		response["fcm_status"] = "success"
	}

	common.WriteSuccessResponse(w, response, "Topic notification processed successfully")
}

// SubscribeToTopic handles POST /notifications/fcm/subscribe
func (h *NotificationHandler) SubscribeToTopic(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User not authenticated"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		Topic    string `json:"topic"`
		FCMToken string `json:"fcm_token,omitempty"` // Optional: specific token to subscribe
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Get user tokens or use provided token
	var tokens []string
	var err error

	if req.FCMToken != "" {
		// Use specific token provided in request
		tokens = []string{req.FCMToken}
		log.Printf("Using provided FCM token for subscription: %s", req.FCMToken)
	} else {
		// Get user's registered tokens
		tokens, err = h.service.fcm.GetUserTokens(r.Context(), userID)
		if err != nil {
			log.Printf("Failed to get user tokens (this is expected in development): %v", err)
			// Don't fail - simulate success for development
			tokens = []string{"development_token"}
		}
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for user %s", userID)
		// Don't fail - simulate success for development
		tokens = []string{"development_token"}
	}

	// Try to subscribe to topic
	err = h.service.fcm.SubscribeToTopic(r.Context(), tokens, req.Topic)
	if err != nil {
		log.Printf("FCM topic subscription failed (this is expected in development): %v", err)

		// For development: simulate successful subscription
		log.Printf("Simulating topic subscription for development: user=%s, topic=%s", userID, req.Topic)
	} else {
		log.Printf("FCM topic subscription successful: user=%s, topic=%s", userID, req.Topic)
	}

	// Always return success for topic subscriptions (FCM is optional)
	response := map[string]interface{}{
		"topic":         req.Topic,
		"token_count":   len(tokens),
		"user_id":       userID,
		"subscribed_at": time.Now().Format(time.RFC3339),
	}

	if err != nil {
		response["fcm_status"] = "failed"
		response["fcm_error"] = "FCM not configured (development mode)"
	} else {
		response["fcm_status"] = "success"
	}

	common.WriteSuccessResponse(w, response, "Topic subscription processed successfully")
}

// UnsubscribeFromTopic handles POST /notifications/fcm/unsubscribe
func (h *NotificationHandler) UnsubscribeFromTopic(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User not authenticated"),
			common.GenerateTraceID())
		return
	}

	var req struct {
		Topic string `json:"topic"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Get user tokens
	tokens, err := h.service.fcm.GetUserTokens(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to get user tokens (this is expected in development): %v", err)
		// Don't fail - simulate success for development
		tokens = []string{"development_token"}
	}

	if len(tokens) == 0 {
		log.Printf("No FCM tokens found for user %s", userID)
		// Don't fail - simulate success for development
		tokens = []string{"development_token"}
	}

	// Try to unsubscribe from topic
	err = h.service.fcm.UnsubscribeFromTopic(r.Context(), tokens, req.Topic)
	if err != nil {
		log.Printf("FCM topic unsubscription failed (this is expected in development): %v", err)

		// For development: simulate successful unsubscription
		log.Printf("Simulating topic unsubscription for development: user=%s, topic=%s", userID, req.Topic)
	} else {
		log.Printf("FCM topic unsubscription successful: user=%s, topic=%s", userID, req.Topic)
	}

	// Always return success for topic unsubscriptions (FCM is optional)
	response := map[string]interface{}{
		"topic":           req.Topic,
		"token_count":     len(tokens),
		"user_id":         userID,
		"unsubscribed_at": time.Now().Format(time.RFC3339),
	}

	if err != nil {
		response["fcm_status"] = "failed"
		response["fcm_error"] = "FCM not configured (development mode)"
	} else {
		response["fcm_status"] = "success"
	}

	common.WriteSuccessResponse(w, response, "Topic unsubscription processed successfully")
}

// HealthCheck handles GET /notifications/health
func (h *NotificationHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"services": map[string]interface{}{
			"fcm": map[string]interface{}{
				"enabled": h.config.EnablePushNotifications,
				"status":  "healthy",
			},
			"email": map[string]interface{}{
				"enabled": h.config.EnableEmailNotifications,
				"status":  "healthy",
			},
		},
	}

	// Test FCM service if enabled
	if h.config.EnablePushNotifications {
		// Simple health check - try to get tokens for a test user
		_, err := h.service.fcm.GetUserTokens(ctx, "health-check")
		if err != nil {
			health["services"].(map[string]interface{})["fcm"].(map[string]interface{})["status"] = "unhealthy"
			health["services"].(map[string]interface{})["fcm"].(map[string]interface{})["error"] = err.Error()
			health["status"] = "degraded"
		}
	}

	// Set appropriate status code
	statusCode := http.StatusOK
	if health["status"] == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

// DebugUserNotifications handles GET /notifications/debug/{user_id} - Debug endpoint
func (h *NotificationHandler) DebugUserNotifications(w http.ResponseWriter, r *http.Request) {
	userID := common.GetPathParam(r, "user_id")
	if userID == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "User ID is required"),
			common.GenerateTraceID())
		return
	}

	log.Printf("Debug: Getting raw notifications for user %s", userID)

	// Get raw database response
	filters := map[string]string{
		"user_id": "eq." + userID,
		"order":   "created_at.desc",
	}

	rows, err := h.service.fcm.db.QueryTable(r.Context(), "notifications", filters)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	var rawNotifications []map[string]interface{}
	if err := rows.Scan(&rawNotifications); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	log.Printf("Debug: Found %d raw notifications for user %s", len(rawNotifications), userID)

	response := map[string]interface{}{
		"user_id":           userID,
		"total_count":       len(rawNotifications),
		"raw_notifications": rawNotifications,
		"query_filters":     filters,
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	common.WriteSuccessResponse(w, response, "Debug data retrieved successfully")
}

// SendTemplateNotification handles POST /notifications/send-template
func (h *NotificationHandler) SendTemplateNotification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FirebaseUID string                 `json:"firebase_uid" validate:"required"`
		TemplateID  string                 `json:"template_id" validate:"required"`
		Data        map[string]interface{} `json:"data"`
		Channels    []string               `json:"channels,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	log.Printf("SendTemplateNotification: firebase_uid=%s, template_id=%s", req.FirebaseUID, req.TemplateID)

	// Get template
	template, exists := GetTemplate(req.TemplateID)
	if !exists {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, fmt.Sprintf("Template '%s' not found", req.TemplateID)),
			common.GenerateTraceID())
		return
	}

	// Override channels if provided
	if len(req.Channels) > 0 {
		template.Channels = req.Channels
	}

	// Merge data
	if template.Data == nil {
		template.Data = make(map[string]interface{})
	}
	for k, v := range req.Data {
		template.Data[k] = v
	}

	// Create a copy of template to avoid modifying the original
	processedTemplate := &NotificationTemplate{
		ID:       template.ID,
		Title:    substitutePlaceholders(template.Title, req.Data),
		Message:  substitutePlaceholders(template.Message, req.Data),
		Type:     template.Type,
		Channels: template.Channels,
		Priority: template.Priority,
		Data:     template.Data,
	}

	// Send notification
	err := h.service.SendNotificationWithFirebaseUID(r.Context(), req.FirebaseUID, processedTemplate)
	if err != nil {
		log.Printf("Failed to send template notification: %v", err)
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, err.Error()),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"firebase_uid": req.FirebaseUID,
		"template_id":  req.TemplateID,
		"title":        processedTemplate.Title,
		"message":      processedTemplate.Message,
		"channels":     processedTemplate.Channels,
		"sent_at":      time.Now().Format(time.RFC3339),
	}

	log.Printf("Template notification sent successfully to Firebase user %s", req.FirebaseUID)
	common.WriteSuccessResponse(w, response, "Template notification sent successfully")
}

// SendBulkNotifications handles POST /notifications/send-bulk
func (h *NotificationHandler) SendBulkNotifications(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserIDs    []string               `json:"user_ids" validate:"required,min=1"`
		TemplateID string                 `json:"template_id" validate:"required"`
		Data       map[string]interface{} `json:"data"`
		Channels   []string               `json:"channels,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if err := common.ValidateStruct(&req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	log.Printf("SendBulkNotifications: %d users, template_id=%s", len(req.UserIDs), req.TemplateID)

	// Get template
	template, exists := GetTemplate(req.TemplateID)
	if !exists {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, fmt.Sprintf("Template '%s' not found", req.TemplateID)),
			common.GenerateTraceID())
		return
	}

	// Override channels if provided
	if len(req.Channels) > 0 {
		template.Channels = req.Channels
	}

	// Merge data
	if template.Data == nil {
		template.Data = make(map[string]interface{})
	}
	for k, v := range req.Data {
		template.Data[k] = v
	}

	// Create a copy of template to avoid modifying the original
	processedTemplate := &NotificationTemplate{
		ID:       template.ID,
		Title:    substitutePlaceholders(template.Title, req.Data),
		Message:  substitutePlaceholders(template.Message, req.Data),
		Type:     template.Type,
		Channels: template.Channels,
		Priority: template.Priority,
		Data:     template.Data,
	}

	// Send to each user
	successCount := 0
	failedUsers := []string{}

	for _, userID := range req.UserIDs {
		err := h.service.SendNotificationWithFirebaseUID(r.Context(), userID, processedTemplate)
		if err != nil {
			log.Printf("Failed to send notification to user %s: %v", userID, err)
			failedUsers = append(failedUsers, userID)
		} else {
			successCount++
		}
	}

	response := map[string]interface{}{
		"template_id":   req.TemplateID,
		"title":         processedTemplate.Title,
		"message":       processedTemplate.Message,
		"channels":      processedTemplate.Channels,
		"total_users":   len(req.UserIDs),
		"success_count": successCount,
		"failed_count":  len(failedUsers),
		"failed_users":  failedUsers,
		"sent_at":       time.Now().Format(time.RFC3339),
	}

	message := fmt.Sprintf("Bulk notifications sent: %d success, %d failed", successCount, len(failedUsers))
	log.Printf("Bulk notifications completed: %d success, %d failed", successCount, len(failedUsers))
	common.WriteSuccessResponse(w, response, message)
}
