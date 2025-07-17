package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/dzuura/satu-lemari/domain/queue"
)

// NotificationSender interface untuk menghindari circular import
type NotificationSender interface {
	SendRequestNotification(ctx context.Context, userID uuid.UUID, requestType, status, itemName string, requestID uuid.UUID) error
	SendRequestNotificationWithFirebaseUID(ctx context.Context, firebaseUID, requestType, status, itemName string, requestID uuid.UUID) error
}

// RequestService handles donation and rental requests
type RequestService struct {
	config             *config.Config
	queueService       *queue.QueueService
	httpClient         *http.Client
	notificationSender NotificationSender
}

// NewRequestService creates a new request service instance
func NewRequestService(cfg *config.Config, notificationSender NotificationSender) *RequestService {
	// Initialize HTTP client for Supabase REST API calls
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// TODO: Initialize Redis cache for queue service
	// For now, we'll create a placeholder queue service
	return &RequestService{
		config:             cfg,
		queueService:       nil, // TODO: Initialize with proper Redis cache
		httpClient:         httpClient,
		notificationSender: notificationSender,
	}
}

// CreateRequest handles POST /requests
func (s *RequestService) CreateRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Validate role - only regular users can create requests
	userRole, ok := common.GetUserRoleFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User role not found"),
			common.GenerateTraceID())
		return
	}

	if userRole != "user" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrForbidden, "Only regular users can create requests"),
			common.GenerateTraceID())
		return
	}

	var reqBody map[string]interface{}
	if err := common.ParseJSONBody(r, &reqBody); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Parse item_id and quantity
	itemIDStr, _ := reqBody["item_id"].(string)
	quantity, _ := reqBody["quantity"].(float64)
	reason, _ := reqBody["reason"].(string)

	// Validate item_id
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item_id format"),
			common.GenerateTraceID())
		return
	}

	// Validate item exists and is available
	item, appErr := s.getItemByID(itemID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check if item is available for request
	if !item.IsAvailable() {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrItemNotAvailable, "Item is not available"),
			common.GenerateTraceID())
		return
	}

	// Prevent users from requesting their own items (additional safety check)
	if item.PartnerID == userID {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidOperation, "You cannot request your own item"),
			common.GenerateTraceID())
		return
	}

	// Fetch user profile to get phone/contact info
	userProfile, appErr := s.getUserByID(userID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Require phone to be filled in user profile
	if userProfile.Phone == nil || *userProfile.Phone == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Please complete your profile (phone number) before making a request"),
			common.GenerateTraceID())
		return
	}

	// Require full name to be filled in user profile
	if userProfile.FullName == nil || *userProfile.FullName == "" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Please complete your profile (full name) before making a request"),
			common.GenerateTraceID())
		return
	}

	// Validate request fields based on item type
	var pickupDate *time.Time
	var returnDate *time.Time

	switch item.Type {
	case "donation":
		if int(quantity) < 1 {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "Quantity is required and must be at least 1 for donation"),
				common.GenerateTraceID())
			return
		}
		pickupDate = nil
		returnDate = nil
	case "rental":
		if int(quantity) < 1 {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "Quantity is required and must be at least 1 for rental"),
				common.GenerateTraceID())
			return
		}
		pickupDateStr, _ := reqBody["pickup_date"].(string)
		returnDateStr, _ := reqBody["return_date"].(string)
		if pickupDateStr == "" {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "pickup_date is required for rental (format: yyyy-mm-dd)"),
				common.GenerateTraceID())
			return
		}
		if returnDateStr == "" {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "return_date is required for rental (format: yyyy-mm-dd)"),
				common.GenerateTraceID())
			return
		}
		parsedPickup, err := time.Parse("2006-01-02", pickupDateStr)
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "pickup_date must be in yyyy-mm-dd format"),
				common.GenerateTraceID())
			return
		}
		parsedReturn, err := time.Parse("2006-01-02", returnDateStr)
		if err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "return_date must be in yyyy-mm-dd format"),
				common.GenerateTraceID())
			return
		}
		if parsedPickup.After(parsedReturn) {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidInput, "pickup_date must be before return_date"),
				common.GenerateTraceID())
			return
		}
		pickupDate = &parsedPickup
		returnDate = &parsedReturn
	default:
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid item type"),
			common.GenerateTraceID())
		return
	}

	// Check if user already has a pending request for this item
	existingRequest, err := s.getExistingRequest(userID, itemID)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to check existing requests"),
			common.GenerateTraceID())
		return
	}

	if existingRequest != nil && existingRequest.IsPending() {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "You already have a pending request for this item"),
			common.GenerateTraceID())
		return
	}

	// For donation requests, check weekly quota
	if item.Type == "donation" {
		// Get full user data for quota check
		fullUser, appErr := s.getFullUserByID(userID)
		if appErr != nil {
			appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
			return
		}

		// Check if user has remaining quota
		if !fullUser.CanRequestDonation() {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrQuotaExceeded, "Weekly donation quota exceeded. You can make more donation requests next Monday."),
				common.GenerateTraceID())
			return
		}

		// Update user's weekly quota (increment used quota)
		if err := s.updateUserDonationQuota(userID, 1); err != nil {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInternal, "Failed to update donation quota"),
				common.GenerateTraceID())
			return
		}
	}

	// Create new request
	request := &models.Request{
		ID:            uuid.New(),
		UserID:        userID,
		ItemID:        itemID,
		PartnerID:     item.PartnerID,
		Type:          item.Type, // Use item type (donation/rental)
		Quantity:      int(quantity),
		Reason:        nil,
		ContactInfo:   userProfile.Phone, // Use phone from user profile
		PickupDate:    pickupDate,
		ReturnDate:    returnDate,
		Status:        "pending",
		PriorityScore: s.calculatePriorityScore(userID),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if reason != "" {
		reasonCopy := reason
		request.Reason = &reasonCopy
	}

	// Save request to database
	if err := s.createRequest(request); err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	// Add to queue for partner notification
	// TODO: Implement queue integration when Redis cache is available
	if s.queueService != nil {
		// queueItem := &queue.QueueRequest{
		//     ID:            uuid.New(),
		//     RequestID:     request.ID,
		//     ItemID:        request.ItemID,
		//     UserID:        request.UserID,
		//     PartnerID:     request.PartnerID,
		//     Type:          request.Type,
		//     PriorityScore: request.PriorityScore,
		//     CreatedAt:     time.Now(),
		// }
		// if err := s.queueService.AddToQueue(r.Context(), queueItem); err != nil {
		//     log.Printf("Failed to add request to queue: %v", err)
		// }
	}

	// Send notifications for request creation
	if s.notificationSender != nil {
		go func() {
			// Get item name for notification
			itemName := "Item"
			if name, err := s.getItemNameByID(request.ItemID.String()); err == nil {
				itemName = name
			}

			// 1. Send confirmation notification to USER (who created the request)
			// Firebase UID is string, not UUID - use new method that accepts Firebase UID
			err := s.notificationSender.SendRequestNotificationWithFirebaseUID(
				context.Background(),
				request.UserID, // Firebase UID as string
				request.Type,
				"created",
				itemName,
				request.ID,
			)
			if err != nil {
				log.Printf("Failed to send request creation notification to user: %v", err)
			} else {
				log.Printf("Successfully sent request creation notification to user %s", request.UserID)
			}

			// 2. Send new request notification to PARTNER (who owns the item)
			err = s.notificationSender.SendRequestNotificationWithFirebaseUID(
				context.Background(),
				request.PartnerID, // Firebase UID as string
				request.Type,
				"new_request", // Different status for partner
				itemName,
				request.ID,
			)
			if err != nil {
				log.Printf("Failed to send new request notification to partner: %v", err)
			} else {
				log.Printf("Successfully sent new request notification to partner %s", request.PartnerID)
			}
		}()
	}

	// Get created request with relations
	createdRequest, appErr := s.getRequestWithRelations(request.ID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteCreatedResponse(w, createdRequest, "Request created successfully")
}

// GetMyRequests handles GET /requests/my
func (s *RequestService) GetMyRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Validate role - only regular users can access my requests
	userRole, ok := common.GetUserRoleFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User role not found"),
			common.GenerateTraceID())
		return
	}

	if userRole != "user" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrForbidden, "Only regular users can access my requests"),
			common.GenerateTraceID())
		return
	}

	filters := s.parseRequestFilters(r)
	filters.UserID = &userID

	pagination := common.GetPaginationParams(r)
	requests, total, err := s.searchRequests(filters, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, requests, meta, "My requests retrieved successfully")
}

// GetPartnerRequests handles GET /requests/partner
func (s *RequestService) GetPartnerRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Validate role - only partners can access partner requests
	userRole, ok := common.GetUserRoleFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "User role not found"),
			common.GenerateTraceID())
		return
	}

	if userRole != "partner" {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrForbidden, "Only partners can access partner requests"),
			common.GenerateTraceID())
		return
	}

	filters := s.parseRequestFilters(r)
	filters.PartnerID = &userID

	pagination := common.GetPaginationParams(r)
	requests, total, err := s.searchRequests(filters, pagination)
	if err != nil {
		appError.WriteErrorResponse(w, err, common.GenerateTraceID())
		return
	}

	meta := common.CalculateMeta(pagination.Page, pagination.Limit, total)
	common.WriteSuccessResponseWithMeta(w, requests, meta, "Partner requests retrieved successfully")
}

// GetRequestByID handles GET /requests/{request_id}
func (s *RequestService) GetRequestByID(w http.ResponseWriter, r *http.Request) {
	requestIDStr := common.GetPathParam(r, "request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request ID"),
			common.GenerateTraceID())
		return
	}

	request, appErr := s.getRequestWithRelations(requestID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, request, "Request retrieved successfully")
}

// UpdateRequest handles PUT /requests/{request_id}
func (s *RequestService) UpdateRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	requestIDStr := common.GetPathParam(r, "request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request ID"),
			common.GenerateTraceID())
		return
	}

	var req models.UpdateRequestRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Get existing request
	existingRequest, appErr := s.getRequestByID(requestID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check authorization (only request owner or partner can update)
	if existingRequest.UserID != userID && existingRequest.PartnerID != userID {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrForbidden, "Access denied"),
			common.GenerateTraceID())
		return
	}

	// Validate status changes based on business logic
	oldStatus := existingRequest.Status
	var newStatus string
	statusChanged := false

	if req.Status != nil {
		newStatus = *req.Status
		if !s.isValidStatusTransition(oldStatus, newStatus) {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidOperation, "Invalid status transition"),
				common.GenerateTraceID())
			return
		}

		// Check if status actually changed
		if oldStatus != newStatus {
			statusChanged = true
			existingRequest.Status = newStatus
		}
	}

	// Validate required fields for specific status changes
	if existingRequest.Status == "rejected" && (req.RejectionReason == nil || *req.RejectionReason == "") {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Rejection reason is required when rejecting a request"),
			common.GenerateTraceID())
		return
	}

	// Handle quota restoration for rejected donation requests
	if existingRequest.Status == "rejected" && oldStatus == "pending" && existingRequest.Type == "donation" {
		// Restore user's weekly quota (decrement used quota)
		log.Printf("Restoring donation quota for rejected request: user=%s, request=%s",
			existingRequest.UserID, existingRequest.ID.String())
		if err := s.updateUserDonationQuota(existingRequest.UserID, -1); err != nil {
			log.Printf("Failed to restore donation quota: %v", err)
			// Don't fail the request update, just log the error
		}
	}

	// Update stok jika status berubah
	if existingRequest.Status == "approved" && oldStatus == "pending" {
		// Kurangi stok item untuk semua jenis request (donation & rental)
		log.Printf("Request approved: decreasing stock for %s request (item: %s, quantity: %d)",
			existingRequest.Type, existingRequest.ItemID.String(), existingRequest.Quantity)
		err := s.updateItemStock(existingRequest.ItemID, -existingRequest.Quantity)
		if err != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to update item stock (decrement)"), common.GenerateTraceID())
			return
		}
	}

	// Stock hanya bertambah untuk RENTAL yang completed/returned, TIDAK untuk donation
	if (existingRequest.Status == "completed" || existingRequest.Status == "returned") && (oldStatus == "approved") {
		if existingRequest.Type == "rental" {
			// Tambah stok item hanya untuk rental (item dikembalikan)
			log.Printf("Rental completed/returned: increasing stock for rental request (item: %s, quantity: %d)",
				existingRequest.ItemID.String(), existingRequest.Quantity)
			err := s.updateItemStock(existingRequest.ItemID, existingRequest.Quantity)
			if err != nil {
				appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to update item stock (increment)"), common.GenerateTraceID())
				return
			}
		} else if existingRequest.Type == "donation" {
			// Untuk donation completed: stock TIDAK bertambah (item sudah diberikan permanen)
			log.Printf("Donation completed: stock remains decreased for donation request (item: %s, quantity: %d)",
				existingRequest.ItemID.String(), existingRequest.Quantity)
		}
	}

	// Update other fields
	if req.RejectionReason != nil {
		existingRequest.RejectionReason = req.RejectionReason
	}
	if req.PickupDate != nil {
		existingRequest.PickupDate = req.PickupDate
	}
	if req.ReturnDate != nil {
		existingRequest.ReturnDate = req.ReturnDate
	}
	existingRequest.UpdatedAt = time.Now()

	// Save updated request
	if appErr := s.updateRequest(existingRequest); appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Send notification to user about request status update
	if statusChanged && s.notificationSender != nil {
		go func() {
			// Get item name for notification
			itemName := "Item"
			if name, err := s.getItemNameByID(existingRequest.ItemID.String()); err == nil {
				itemName = name
			}

			log.Printf("Sending request status update notification: user=%s, status=%s->%s, item=%s, requestID=%s",
				existingRequest.UserID, oldStatus, newStatus, itemName, existingRequest.ID.String())

			// Use Firebase UID method for notification
			err := s.notificationSender.SendRequestNotificationWithFirebaseUID(
				context.Background(),
				existingRequest.UserID, // Firebase UID as string
				existingRequest.Type,
				newStatus, // Use the new status
				itemName,
				existingRequest.ID,
			)
			if err != nil {
				log.Printf("Failed to send request status update notification: %v", err)
			} else {
				log.Printf("Successfully sent request status update notification to user %s", existingRequest.UserID)
			}
		}()
	}

	// Get updated request with relations
	updatedRequest, appErr := s.getRequestWithRelations(requestID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	common.WriteSuccessResponse(w, updatedRequest, "Request updated successfully")
}

// isValidStatusTransition validates if the status transition is allowed
func (s *RequestService) isValidStatusTransition(currentStatus, newStatus string) bool {
	// Define valid status transitions
	validTransitions := map[string][]string{
		"pending":   {"approved", "rejected"},
		"approved":  {"completed", "returned"},
		"rejected":  {}, // No further transitions allowed
		"completed": {}, // No further transitions allowed
		"returned":  {}, // No further transitions allowed
	}

	allowedTransitions, exists := validTransitions[currentStatus]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == newStatus {
			return true
		}
	}

	return false
}

// DeleteRequest handles DELETE /requests/{request_id}
func (s *RequestService) DeleteRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	requestIDStr := common.GetPathParam(r, "request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request ID"),
			common.GenerateTraceID())
		return
	}

	// Get existing request
	existingRequest, appErr := s.getRequestByID(requestID)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check authorization (request owner or partner can delete)
	var deletedBy string
	if existingRequest.UserID == userID {
		deletedBy = "user"
	} else if existingRequest.PartnerID == userID {
		deletedBy = "partner"
	} else {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrForbidden, "Access denied: only request owner or partner can delete this request"),
			common.GenerateTraceID())
		return
	}

	// Check if request can be deleted based on who is deleting and implement soft delete logic
	var deleteType string
	if deletedBy == "user" {
		// User can delete request with any status
		if existingRequest.Status == "pending" {
			// Pending requests: hard delete (removes from both user and partner view)
			deleteType = "hard_delete"
			log.Printf("DeleteRequest: User deleting pending request - will be hard deleted (removed from both views)")
		} else {
			// Non-pending requests: soft delete (only hide from user view)
			deleteType = "soft_delete_user"
			log.Printf("DeleteRequest: User deleting %s request - will be soft deleted (hidden from user only)", existingRequest.Status)
		}
	} else if deletedBy == "partner" {
		// Partner can only delete completed requests
		if existingRequest.Status != "completed" {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrInvalidOperation, "Partners can only delete completed requests"),
				common.GenerateTraceID())
			return
		}
		// Completed requests: soft delete (only hide from partner view)
		deleteType = "soft_delete_partner"
		log.Printf("DeleteRequest: Partner deleting completed request - will be soft deleted (hidden from partner only)")
	}

	log.Printf("DeleteRequest: %s (ID: %s) is deleting request %s for item %s (delete_type: %s)",
		deletedBy, userID, requestID.String(), existingRequest.ItemID.String(), deleteType)

	// Execute delete based on type
	if appErr := s.deleteRequestWithType(requestID, deleteType); appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Return the deleted request data for confirmation
	response := map[string]interface{}{
		"deleted_request_id": requestID.String(),
		"deleted_by":         deletedBy,
		"deleted_by_user_id": userID,
		"deleted_at":         time.Now().Format(time.RFC3339),
		"request_type":       existingRequest.Type,
		"item_id":            existingRequest.ItemID.String(),
		"delete_type":        deleteType,
	}

	var message string
	switch deleteType {
	case "hard_delete":
		message = fmt.Sprintf("Request permanently deleted by %s (removed from both user and partner view)", deletedBy)
	case "soft_delete_user":
		message = fmt.Sprintf("Request hidden from user view by %s (still visible to partner)", deletedBy)
	case "soft_delete_partner":
		message = fmt.Sprintf("Request hidden from partner view by %s (still visible to user)", deletedBy)
	default:
		message = fmt.Sprintf("Request deleted successfully by %s", deletedBy)
	}

	log.Printf("DeleteRequest: Successfully processed delete request %s by %s (ID: %s, type: %s)",
		requestID.String(), deletedBy, userID, deleteType)

	common.WriteSuccessResponse(w, response, message)
}

// parseRequestFilters parses request filters from query parameters
func (s *RequestService) parseRequestFilters(r *http.Request) *models.RequestFilter {
	filters := &models.RequestFilter{}

	if typeFilter := r.URL.Query().Get("type"); typeFilter != "" {
		filters.Type = &typeFilter
	}
	if statusFilter := r.URL.Query().Get("status"); statusFilter != "" {
		filters.Status = &statusFilter
	}
	if itemIDStr := r.URL.Query().Get("item_id"); itemIDStr != "" {
		if itemID, err := uuid.Parse(itemIDStr); err == nil {
			filters.ItemID = &itemID
		}
	}
	if dateFromStr := r.URL.Query().Get("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse("2006-01-02", dateFromStr); err == nil {
			filters.DateFrom = &dateFrom
		}
	}
	if dateToStr := r.URL.Query().Get("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse("2006-01-02", dateToStr); err == nil {
			filters.DateTo = &dateTo
		}
	}
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		filters.SortBy = &sortBy
	}
	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		filters.SortOrder = &sortOrder
	}

	return filters
}

// calculatePriorityScore calculates priority score for request
func (s *RequestService) calculatePriorityScore(userID string) float64 {
	// TODO: Implement proper priority calculation based on:
	// - User history (completed requests)
	// - User rating
	// - Request urgency
	// - Item availability

	// For now, return a basic score
	completedRequests := 0
	totalRequests := 0

	if c, err := s.getUserCompletedRequests(userID); err == nil {
		completedRequests = c
	} else {
		log.Printf("Failed to get user completed requests: %v", err)
	}

	if t, err := s.getUserTotalRequests(userID); err == nil {
		totalRequests = t
	} else {
		log.Printf("Failed to get user total requests: %v", err)
	}

	// Base score starts at 1.0
	score := 1.0

	// Bonus for users with good history
	if totalRequests > 0 {
		completionRate := float64(completedRequests) / float64(totalRequests)
		score += completionRate * 0.5 // Up to 0.5 bonus for good completion rate
	}

	return score
}

// createRequest saves a new request to the database
func (s *RequestService) createRequest(request *models.Request) *appError.AppError {
	url := fmt.Sprintf("%s/rest/v1/requests", s.config.SupabaseURL)

	// Prepare request data for Supabase
	requestData := map[string]interface{}{
		"id":                 request.ID.String(),
		"item_id":            request.ItemID.String(),
		"user_id":            request.UserID,
		"partner_id":         request.PartnerID,
		"type":               request.Type,
		"quantity":           request.Quantity,
		"reason":             request.Reason,
		"contact_info":       request.ContactInfo,
		"status":             request.Status,
		"priority_score":     request.PriorityScore,
		"deleted_by_user":    false,
		"deleted_by_partner": false,
		"created_at":         request.CreatedAt.Format(time.RFC3339),
		"updated_at":         request.UpdatedAt.Format(time.RFC3339),
	}

	// Handle date fields properly - format as date string or null
	if request.PickupDate != nil {
		requestData["pickup_date"] = request.PickupDate.Format("2006-01-02")
	} else {
		requestData["pickup_date"] = nil
	}

	if request.ReturnDate != nil {
		requestData["return_date"] = request.ReturnDate.Format("2006-01-02")
	} else {
		requestData["return_date"] = nil
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal request data")
	}

	log.Printf("Creating request with data: %s", string(jsonData))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	// Use service role key to bypass RLS (konsisten dengan modul user)
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return appError.New(appError.ErrDatabase, "Failed to create request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to create request: %s", string(body))
		return appError.New(appError.ErrDatabase, "Failed to create request in database")
	}

	log.Printf("Request created successfully with ID: %s", request.ID.String())
	return nil
}

// getRequestByID retrieves a request by ID
func (s *RequestService) getRequestByID(requestID uuid.UUID) (*models.Request, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/requests?id=eq.%s&limit=1", s.config.SupabaseURL, requestID.String())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to retrieve request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Supabase getRequestByID error: %s | url: %s", string(body), url)
		return nil, appError.New(appError.ErrNotFound, "Request not found")
	}

	// Read response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to read response body")
	}

	log.Printf("Supabase getRequestByID response: %s", string(body))

	var requests []models.Request
	if err := json.Unmarshal(body, &requests); err != nil {
		log.Printf("Failed to decode response: %v | Response body: %s", err, string(body))
		return nil, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	if len(requests) == 0 {
		return nil, appError.New(appError.ErrNotFound, "Request not found")
	}

	return &requests[0], nil
}

// getRequestWithRelations retrieves a request with related data
func (s *RequestService) getRequestWithRelations(requestID uuid.UUID) (*models.Request, *appError.AppError) {
	request, err := s.getRequestByID(requestID)
	if err != nil {
		return nil, err
	}

	// Load related item with additional details
	if item, err := s.getItemByID(request.ItemID); err == nil {
		request.Item = item
		request.ItemName = item.Name
		request.ItemImages = item.Images

		// Include price only for rental type
		if request.Type == "rental" && item.Price != nil {
			request.ItemPrice = item.Price
		}

		// Load category name
		if category, err := s.getCategoryByID(item.CategoryID); err == nil {
			request.CategoryName = category.Name
			// Also populate category in item relation
			item.Category = category
		}
	}

	// Load related user with additional details
	if user, err := s.getUserByID(request.UserID); err == nil {
		request.User = user
		request.UserName = user.Username
		if user.FullName != nil {
			request.UserFullName = *user.FullName
		}
		request.UserPhone = user.Phone
		request.UserPhoto = user.Photo
	}

	// Load related partner
	if partner, err := s.getUserByID(request.PartnerID); err == nil {
		request.Partner = partner
	}

	return request, nil
}

// searchRequests searches for requests with filters and pagination
func (s *RequestService) searchRequests(filters *models.RequestFilter, pagination common.PaginationParams) ([]models.Request, int, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/requests?%s", s.config.SupabaseURL, buildRequestQueryString(filters, pagination))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Supabase HTTP error: %v", err)
		return nil, 0, appError.New(appError.ErrDatabase, "Failed to search requests")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Supabase searchRequests error: %s | url: %s", string(body), url)
		return nil, 0, appError.New(appError.ErrDatabase, "Failed to search requests")
	}

	var requests []models.Request
	if err := json.NewDecoder(resp.Body).Decode(&requests); err != nil {
		return nil, 0, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	// Populate additional information for each request
	for i := range requests {
		// Get item details (name, price, images, category)
		if item, err := s.getItemByID(requests[i].ItemID); err == nil {
			requests[i].ItemName = item.Name
			requests[i].ItemImages = item.Images

			// Include price only for rental type
			if requests[i].Type == "rental" && item.Price != nil {
				requests[i].ItemPrice = item.Price
			}

			// Get category name
			if category, err := s.getCategoryByID(item.CategoryID); err == nil {
				requests[i].CategoryName = category.Name
				log.Printf("Successfully got category name: %s for category ID: %s", category.Name, item.CategoryID.String())
			} else {
				log.Printf("Failed to get category name for category ID: %s, error: %v", item.CategoryID.String(), err)
			}

			log.Printf("Successfully got item details: name=%s, price=%v, images_count=%d, category=%s for item ID: %s",
				item.Name, item.Price, len(item.Images), requests[i].CategoryName, requests[i].ItemID.String())
		} else {
			log.Printf("Failed to get item details for item ID: %s, error: %v", requests[i].ItemID.String(), err)
		}

		// Get user info (username, full name, phone, and photo) in one query
		if userInfo, err := s.getUserInfoByID(requests[i].UserID); err == nil {
			requests[i].UserName = userInfo.Username
			requests[i].UserFullName = userInfo.FullName
			requests[i].UserPhone = userInfo.Phone
			requests[i].UserPhoto = userInfo.Photo
			log.Printf("Successfully got user info: username=%s, full_name=%s, phone=%v, photo=%v for user ID: %s",
				userInfo.Username, userInfo.FullName, userInfo.Phone, userInfo.Photo, requests[i].UserID)
		} else {
			log.Printf("Failed to get user info for user ID: %s, error: %v", requests[i].UserID, err)
		}
	}

	// Get total count
	total, err := s.getRequestsCount(filters)
	if err != nil {
		log.Printf("Failed to get requests count: %v", err)
		total = len(requests) // Fallback to current page count
	}

	return requests, total, nil
}

// updateRequest updates an existing request
func (s *RequestService) updateRequest(request *models.Request) *appError.AppError {
	url := fmt.Sprintf("%s/rest/v1/requests?id=eq.%s", s.config.SupabaseURL, request.ID.String())

	// Prepare update data
	updateData := map[string]interface{}{
		"status":           request.Status, // pastikan status selalu dikirim
		"rejection_reason": request.RejectionReason,
		"pickup_date":      request.PickupDate,
		"return_date":      request.ReturnDate,
		"updated_at":       request.UpdatedAt.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Supabase PATCH error: %v", err)
		return appError.New(appError.ErrDatabase, "Failed to update request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Supabase PATCH response: status=%d, body=%s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return appError.New(appError.ErrDatabase, "Failed to update request in database")
	}

	return nil
}

// deleteRequestWithType handles different types of delete operations
func (s *RequestService) deleteRequestWithType(requestID uuid.UUID, deleteType string) *appError.AppError {
	switch deleteType {
	case "hard_delete":
		return s.hardDeleteRequest(requestID)
	case "soft_delete_user":
		return s.softDeleteRequest(requestID, true, false)
	case "soft_delete_partner":
		return s.softDeleteRequest(requestID, false, true)
	default:
		return appError.New(appError.ErrInternal, "Invalid delete type")
	}
}

// hardDeleteRequest permanently deletes a request from database
func (s *RequestService) hardDeleteRequest(requestID uuid.UUID) *appError.AppError {
	url := fmt.Sprintf("%s/rest/v1/requests?id=eq.%s", s.config.SupabaseURL, requestID.String())
	log.Printf("Hard deleting request with URL: %s", url)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.Printf("Failed to create DELETE request: %v", err)
		return appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to execute DELETE request: %v", err)
		return appError.New(appError.ErrDatabase, "Failed to delete request")
	}
	defer resp.Body.Close()

	log.Printf("Hard DELETE request response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Hard DELETE request failed with status %d: %s", resp.StatusCode, string(body))
		return appError.New(appError.ErrDatabase, "Failed to delete request from database")
	}

	log.Printf("Request hard deleted successfully: %s", requestID.String())
	return nil
}

// softDeleteRequest marks a request as deleted for specific user type
func (s *RequestService) softDeleteRequest(requestID uuid.UUID, deletedByUser, deletedByPartner bool) *appError.AppError {
	url := fmt.Sprintf("%s/rest/v1/requests?id=eq.%s", s.config.SupabaseURL, requestID.String())

	// Prepare update data
	updateData := map[string]interface{}{
		"updated_at": time.Now().Format(time.RFC3339),
	}

	if deletedByUser {
		updateData["deleted_by_user"] = true
	}
	if deletedByPartner {
		updateData["deleted_by_partner"] = true
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to marshal update data")
	}

	log.Printf("Soft deleting request with URL: %s, data: %s", url, string(jsonData))

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Supabase PATCH error: %v", err)
		return appError.New(appError.ErrDatabase, "Failed to soft delete request")
	}
	defer resp.Body.Close()

	log.Printf("Soft DELETE request response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Soft DELETE request failed with status %d: %s", resp.StatusCode, string(body))
		return appError.New(appError.ErrDatabase, "Failed to soft delete request")
	}

	log.Printf("Request soft deleted successfully: %s (user: %v, partner: %v)",
		requestID.String(), deletedByUser, deletedByPartner)
	return nil
}

// deleteRequest - legacy method for backward compatibility
func (s *RequestService) deleteRequest(requestID uuid.UUID) *appError.AppError {
	return s.hardDeleteRequest(requestID)
}

// getExistingRequest checks if user already has a request for the item
func (s *RequestService) getExistingRequest(userID string, itemID uuid.UUID) (*models.Request, error) {
	url := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&item_id=eq.%s&limit=1", s.config.SupabaseURL, userID, itemID.String())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to check existing request")
	}

	var requests []models.Request
	if err := json.NewDecoder(resp.Body).Decode(&requests); err != nil {
		return nil, err
	}

	if len(requests) == 0 {
		return nil, nil // No existing request
	}

	return &requests[0], nil
}

// getRequestsCount gets total count of requests matching filters
func (s *RequestService) getRequestsCount(filters *models.RequestFilter) (int, error) {
	url := fmt.Sprintf("%s/rest/v1/requests?%s&select=id", s.config.SupabaseURL, buildRequestCountQueryString(filters))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Supabase HTTP error (count): %v", err)
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Supabase getRequestsCount error: %s | url: %s", string(body), url)
		return 0, fmt.Errorf("supabase error: %s", string(body))
	}

	// Parse count from response
	var countData []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&countData); err != nil {
		return 0, err
	}

	if len(countData) == 0 {
		return 0, nil
	}

	if count, ok := countData[0]["count"]; ok {
		if countFloat, ok := count.(float64); ok {
			return int(countFloat), nil
		}
	}

	return 0, fmt.Errorf("invalid count format")
}

// getUserCompletedRequests gets count of completed requests for user
func (s *RequestService) getUserCompletedRequests(userID string) (int, error) {
	url := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&status=eq.completed&select=count", s.config.SupabaseURL, userID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get completed requests count")
	}

	var countData []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&countData); err != nil {
		return 0, err
	}

	if len(countData) == 0 {
		return 0, nil
	}

	if count, ok := countData[0]["count"]; ok {
		if countFloat, ok := count.(float64); ok {
			return int(countFloat), nil
		}
	}

	return 0, nil
}

// getUserTotalRequests gets total count of requests for user
func (s *RequestService) getUserTotalRequests(userID string) (int, error) {
	url := fmt.Sprintf("%s/rest/v1/requests?user_id=eq.%s&select=count", s.config.SupabaseURL, userID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get total requests count")
	}

	var countData []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&countData); err != nil {
		return 0, err
	}

	if len(countData) == 0 {
		return 0, nil
	}

	if count, ok := countData[0]["count"]; ok {
		if countFloat, ok := count.(float64); ok {
			return int(countFloat), nil
		}
	}

	return 0, nil
}

// getItemByID retrieves an item by ID using Supabase REST API
func (s *RequestService) getItemByID(itemID uuid.UUID) (*models.Item, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&limit=1", s.config.SupabaseURL, itemID.String())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to retrieve item")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrNotFound, "Item not found")
	}

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Supabase getItemByID response: %s", string(body))
	var items []models.Item
	if err := json.Unmarshal(body, &items); err != nil {
		log.Printf("Failed to decode Supabase item response: %v", err)
		return nil, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	if len(items) == 0 {
		return nil, appError.New(appError.ErrNotFound, "Item not found")
	}

	return &items[0], nil
}

// getCategoryByID retrieves a category by ID using Supabase REST API
func (s *RequestService) getCategoryByID(categoryID uuid.UUID) (*models.Category, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/categories?id=eq.%s&limit=1", s.config.SupabaseURL, categoryID.String())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to retrieve category")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrNotFound, "Category not found")
	}

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Supabase getCategoryByID response: %s", string(body))
	var categories []models.Category
	if err := json.Unmarshal(body, &categories); err != nil {
		log.Printf("Failed to decode Supabase category response: %v", err)
		return nil, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	if len(categories) == 0 {
		return nil, appError.New(appError.ErrNotFound, "Category not found")
	}

	return &categories[0], nil
}

// getUserByID retrieves a user profile by ID using Supabase REST API
func (s *RequestService) getUserByID(userID string) (*models.UserProfile, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&limit=1", s.config.SupabaseURL, userID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to retrieve user")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrNotFound, "User not found")
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	if len(users) == 0 {
		return nil, appError.New(appError.ErrNotFound, "User not found")
	}

	userProfile := users[0].ToProfile()
	return &userProfile, nil
}

// updateItemStock menambah/mengurangi available_quantity pada item
func (s *RequestService) updateItemStock(itemID uuid.UUID, delta int) error {
	log.Printf("Updating item stock for item %s with delta %d", itemID.String(), delta)

	// Ambil item dulu
	urlGet := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=available_quantity", s.config.SupabaseURL, itemID.String())
	reqGet, err := http.NewRequest("GET", urlGet, nil)
	if err != nil {
		log.Printf("Failed to create GET request: %v", err)
		return err
	}
	reqGet.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	reqGet.Header.Set("apikey", s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(reqGet)
	if err != nil {
		log.Printf("Failed to execute GET request: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("GET item stock failed with status %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("failed to get item stock")
	}

	// Read and log response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return err
	}
	log.Printf("Item stock response: %s", string(body))

	var items []struct {
		AvailableQuantity int `json:"available_quantity"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		log.Printf("Failed to decode item stock response: %v", err)
		return err
	}

	if len(items) == 0 {
		log.Printf("Item not found for ID: %s", itemID.String())
		return fmt.Errorf("item not found")
	}

	currentQty := items[0].AvailableQuantity
	newQty := currentQty + delta
	log.Printf("Current quantity: %d, Delta: %d, New quantity: %d", currentQty, delta, newQty)

	if newQty < 0 {
		log.Printf("Stock would become negative: %d", newQty)
		return fmt.Errorf("stock cannot be negative")
	}

	// Update stok
	urlPatch := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID.String())
	patchData := map[string]interface{}{"available_quantity": newQty}
	jsonData, _ := json.Marshal(patchData)
	log.Printf("Updating item stock with data: %s", string(jsonData))

	reqPatch, err := http.NewRequest("PATCH", urlPatch, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create PATCH request: %v", err)
		return err
	}
	reqPatch.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	reqPatch.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	reqPatch.Header.Set("Content-Type", "application/json")

	resp2, err := s.httpClient.Do(reqPatch)
	if err != nil {
		log.Printf("Failed to execute PATCH request: %v", err)
		return err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK && resp2.StatusCode != http.StatusNoContent {
		body2, _ := io.ReadAll(resp2.Body)
		log.Printf("PATCH item stock failed with status %d: %s", resp2.StatusCode, string(body2))
		return fmt.Errorf("failed to update item stock")
	}

	log.Printf("Item stock updated successfully")
	return nil
}

// Helper: buildRequestQueryString dan buildRequestCountQueryString pastikan filter partner_id/user_id benar
func buildRequestQueryString(filters *models.RequestFilter, pagination common.PaginationParams) string {
	params := url.Values{}
	if filters.Type != nil {
		params.Set("type", "eq."+*filters.Type)
	}
	if filters.Status != nil {
		params.Set("status", "eq."+*filters.Status)
	}
	if filters.ItemID != nil {
		params.Set("item_id", "eq."+filters.ItemID.String())
	}
	if filters.DateFrom != nil {
		params.Set("created_at", "gte."+filters.DateFrom.Format("2006-01-02"))
	}
	if filters.DateTo != nil {
		params.Set("created_at", "lte."+filters.DateTo.Format("2006-01-02"))
	}
	if filters.UserID != nil {
		params.Set("user_id", "eq."+*filters.UserID)
		// For user requests: exclude requests deleted by user
		params.Set("deleted_by_user", "eq.false")
	}
	if filters.PartnerID != nil {
		params.Set("partner_id", "eq."+*filters.PartnerID)
		// For partner requests: exclude requests deleted by partner
		params.Set("deleted_by_partner", "eq.false")
	}
	params.Set("order", "created_at.desc")
	params.Set("limit", fmt.Sprintf("%d", pagination.Limit))
	params.Set("offset", fmt.Sprintf("%d", (pagination.Page-1)*pagination.Limit))
	return params.Encode()
}

func buildRequestCountQueryString(filters *models.RequestFilter) string {
	params := url.Values{}
	if filters.Type != nil {
		params.Set("type", "eq."+*filters.Type)
	}
	if filters.Status != nil {
		params.Set("status", "eq."+*filters.Status)
	}
	if filters.ItemID != nil {
		params.Set("item_id", "eq."+filters.ItemID.String())
	}
	if filters.DateFrom != nil {
		params.Set("created_at", "gte."+filters.DateFrom.Format("2006-01-02"))
	}
	if filters.DateTo != nil {
		params.Set("created_at", "lte."+filters.DateTo.Format("2006-01-02"))
	}
	if filters.UserID != nil {
		params.Set("user_id", "eq."+*filters.UserID)
		// For user requests: exclude requests deleted by user
		params.Set("deleted_by_user", "eq.false")
	}
	if filters.PartnerID != nil {
		params.Set("partner_id", "eq."+*filters.PartnerID)
		// For partner requests: exclude requests deleted by partner
		params.Set("deleted_by_partner", "eq.false")
	}
	params.Set("select", "count")
	return params.Encode()
}

// getItemNameByID retrieves item name by ID from Supabase
func (s *RequestService) getItemNameByID(itemID string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=name&limit=1",
		s.config.SupabaseURL, itemID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get item")
	}

	var items []struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return "", err
	}

	if len(items) == 0 {
		return "", fmt.Errorf("item not found")
	}

	return items[0].Name, nil
}

// UserInfo holds user information for requests
type UserInfo struct {
	Username string  `json:"username"`
	FullName string  `json:"full_name"`
	Phone    *string `json:"phone"`
	Photo    *string `json:"photo"`
}

// getUserInfoByID retrieves user info (username and full name) by ID from Supabase
func (s *RequestService) getUserInfoByID(userID string) (*UserInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Query using correct column name from schema (id, not firebase_uid)
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=username,full_name,phone,photo&limit=1",
		s.config.SupabaseURL, userID)

	log.Printf("Getting user info with URL: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("User info response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user: status %d, body: %s", resp.StatusCode, string(body))
	}

	var users []UserInfo
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

// getFullUserByID retrieves a full user model by ID (with quota fields)
func (s *RequestService) getFullUserByID(userID string) (*models.User, *appError.AppError) {
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&limit=1", s.config.SupabaseURL, userID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to create HTTP request")
	}

	// Use service role key to bypass RLS
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Failed to retrieve user")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrNotFound, "User not found")
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to decode response")
	}

	if len(users) == 0 {
		return nil, appError.New(appError.ErrNotFound, "User not found")
	}

	return &users[0], nil
}

// updateUserDonationQuota updates user's weekly donation quota
func (s *RequestService) updateUserDonationQuota(userID string, delta int) error {
	// Get current user data
	user, appErr := s.getFullUserByID(userID)
	if appErr != nil {
		return fmt.Errorf("failed to get user: %v", appErr)
	}

	// Calculate new quota used
	newQuotaUsed := user.WeeklyDonationUsed + delta
	if newQuotaUsed < 0 {
		newQuotaUsed = 0
	}

	// Update user quota in database
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	updateData := map[string]interface{}{
		"weekly_donation_used": newQuotaUsed,
		"updated_at":           time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal update data: %v", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update user quota: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update user quota: %s", string(body))
	}

	return nil
}
