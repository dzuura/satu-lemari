package queue

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/google/uuid"
)

// QueueService handles request queuing and processing
type QueueService struct {
	cache *cache.RedisCache
}

// QueueRequest represents a request in the queue
type QueueRequest struct {
	ID            uuid.UUID  `json:"id"`
	ItemID        uuid.UUID  `json:"item_id"`
	UserID        uuid.UUID  `json:"user_id"`
	PartnerID     uuid.UUID  `json:"partner_id"`
	Type          string     `json:"type"` // donation or rental
	Quantity      int        `json:"quantity"`
	Reason        string     `json:"reason,omitempty"`
	ContactInfo   string     `json:"contact_info,omitempty"`
	PickupDate    *time.Time `json:"pickup_date,omitempty"`
	ReturnDate    *time.Time `json:"return_date,omitempty"`
	PriorityScore float64    `json:"priority_score"`
	QueuePosition int        `json:"queue_position"`
	CreatedAt     time.Time  `json:"created_at"`
	Status        string     `json:"status"` // pending, processing, completed, cancelled
}

// PriorityFactors represents factors that affect priority scoring
type PriorityFactors struct {
	UserHistoryScore float64 `json:"user_history_score"`  // Based on user's past behavior
	UrgencyScore     float64 `json:"urgency_score"`       // Based on pickup date
	LocationScore    float64 `json:"location_score"`      // Based on distance
	QuotaScore       float64 `json:"quota_score"`         // Based on remaining quota
	TimeInQueueScore float64 `json:"time_in_queue_score"` // Based on how long in queue
	ReasonScore      float64 `json:"reason_score"`        // Based on reason quality
}

// NewQueueService creates a new queue service
func NewQueueService(cache *cache.RedisCache) *QueueService {
	return &QueueService{
		cache: cache,
	}
}

// AddToQueue adds a request to the queue
func (q *QueueService) AddToQueue(ctx context.Context, request *QueueRequest) error {
	// Generate queue key
	queueKey := q.generateQueueKey(request.ItemID, request.Type)

	// Calculate priority score
	priorityScore, err := q.calculatePriorityScore(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to calculate priority score: %v", err)
	}
	request.PriorityScore = priorityScore

	// Get current queue position
	queuePosition, err := q.getQueuePosition(ctx, queueKey)
	if err != nil {
		return fmt.Errorf("failed to get queue position: %v", err)
	}
	request.QueuePosition = queuePosition + 1

	// Add to queue
	err = q.cache.LPush(ctx, queueKey, request)
	if err != nil {
		return fmt.Errorf("failed to add request to queue: %v", err)
	}

	// Set request status
	request.Status = "pending"
	request.CreatedAt = time.Now()

	// Store request details
	requestKey := q.generateRequestKey(request.ID)
	err = q.cache.Set(ctx, requestKey, request, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to store request details: %v", err)
	}

	log.Printf("Request %s added to queue for item %s at position %d with score %.2f",
		request.ID, request.ItemID, request.QueuePosition, request.PriorityScore)

	return nil
}

// GetQueuePosition gets the current position in queue
func (q *QueueService) GetQueuePosition(ctx context.Context, requestID uuid.UUID) (int, error) {
	// Get request details
	requestKey := q.generateRequestKey(requestID)
	var request QueueRequest
	err := q.cache.Get(ctx, requestKey, &request)
	if err != nil {
		return 0, fmt.Errorf("request not found: %v", err)
	}

	return request.QueuePosition, nil
}

// GetQueueStatus gets the current queue status for an item
func (q *QueueService) GetQueueStatus(ctx context.Context, itemID uuid.UUID, requestType string) (*QueueStatus, error) {
	queueKey := q.generateQueueKey(itemID, requestType)

	// Get queue length
	queueLength, err := q.cache.LLen(ctx, queueKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue length: %v", err)
	}

	// Get estimated wait time
	estimatedWaitTime := q.calculateEstimatedWaitTime(queueLength)

	status := &QueueStatus{
		ItemID:            itemID,
		Type:              requestType,
		QueueLength:       queueLength,
		EstimatedWaitTime: estimatedWaitTime,
		IsOpen:            true, // Queue is always open for new requests
	}

	return status, nil
}

// ProcessQueue processes the queue for an item
func (q *QueueService) ProcessQueue(ctx context.Context, itemID uuid.UUID, requestType string, availableQuantity int) ([]*QueueRequest, error) {
	queueKey := q.generateQueueKey(itemID, requestType)

	// Get all requests in queue
	queueLength, err := q.cache.LLen(ctx, queueKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue length: %v", err)
	}

	if queueLength == 0 {
		return []*QueueRequest{}, nil
	}

	// Get all requests from queue
	var requests []*QueueRequest
	for i := int64(0); i < queueLength; i++ {
		var request QueueRequest
		err := q.cache.RPop(ctx, queueKey, &request)
		if err != nil {
			log.Printf("Failed to get request from queue: %v", err)
			continue
		}
		requests = append(requests, &request)
	}

	// Sort by priority score (highest first)
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].PriorityScore > requests[j].PriorityScore
	})

	// Process requests based on available quantity
	var processedRequests []*QueueRequest
	remainingQuantity := availableQuantity

	for _, request := range requests {
		if remainingQuantity <= 0 {
			break
		}

		// Check if request can be fulfilled
		if request.Quantity <= remainingQuantity {
			request.Status = "processing"
			processedRequests = append(processedRequests, request)
			remainingQuantity -= request.Quantity
		} else {
			// Partial fulfillment
			request.Status = "processing"
			request.Quantity = remainingQuantity
			processedRequests = append(processedRequests, request)
			remainingQuantity = 0
		}

		// Update request status
		requestKey := q.generateRequestKey(request.ID)
		err := q.cache.Set(ctx, requestKey, request, 24*time.Hour)
		if err != nil {
			log.Printf("Failed to update request status: %v", err)
		}
	}

	// Re-add unprocessed requests to queue
	for _, request := range requests {
		if request.Status != "processing" {
			request.QueuePosition = 1 // Reset position
			err := q.cache.LPush(ctx, queueKey, request)
		if err != nil {
				log.Printf("Failed to re-add request to queue: %v", err)
			}
		}
	}

	return processedRequests, nil
}

// RemoveFromQueue removes a request from the queue
func (q *QueueService) RemoveFromQueue(ctx context.Context, requestID uuid.UUID) error {
	// Get request details
	requestKey := q.generateRequestKey(requestID)
	var request QueueRequest
	err := q.cache.Get(ctx, requestKey, &request)
	if err != nil {
		return fmt.Errorf("request not found: %v", err)
	}

	// Remove from queue
	queueKey := q.generateQueueKey(request.ItemID, request.Type)

	// Get all requests and filter out the one to remove
	queueLength, err := q.cache.LLen(ctx, queueKey)
	if err != nil {
		return fmt.Errorf("failed to get queue length: %v", err)
	}

	var remainingRequests []*QueueRequest
	for i := int64(0); i < queueLength; i++ {
		var req QueueRequest
		err := q.cache.RPop(ctx, queueKey, &req)
		if err != nil {
			continue
		}

		if req.ID != requestID {
			remainingRequests = append(remainingRequests, &req)
		}
	}

	// Re-add remaining requests
	for i, req := range remainingRequests {
		req.QueuePosition = i + 1
		err := q.cache.LPush(ctx, queueKey, req)
		if err != nil {
			log.Printf("Failed to re-add request to queue: %v", err)
		}
	}

	// Delete request details
	err = q.cache.Delete(ctx, requestKey)
	if err != nil {
		log.Printf("Failed to delete request details: %v", err)
	}

	return nil
}

// UpdateRequestStatus updates the status of a request
func (q *QueueService) UpdateRequestStatus(ctx context.Context, requestID uuid.UUID, status string) error {
	requestKey := q.generateRequestKey(requestID)
	var request QueueRequest
	err := q.cache.Get(ctx, requestKey, &request)
	if err != nil {
		return fmt.Errorf("request not found: %v", err)
	}

	request.Status = status
	err = q.cache.Set(ctx, requestKey, request, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to update request status: %v", err)
	}

	return nil
}

// calculatePriorityScore calculates the priority score for a request
func (q *QueueService) calculatePriorityScore(ctx context.Context, request *QueueRequest) (float64, error) {
	factors := &PriorityFactors{}

	// User history score (0-25 points)
	userHistoryScore, err := q.calculateUserHistoryScore(ctx, request.UserID)
	if err != nil {
		log.Printf("Failed to calculate user history score: %v", err)
		userHistoryScore = 10 // Default score
	}
	factors.UserHistoryScore = userHistoryScore

	// Urgency score (0-20 points)
	urgencyScore := q.calculateUrgencyScore(request.PickupDate)
	factors.UrgencyScore = urgencyScore

	// Location score (0-15 points)
	locationScore, err := q.calculateLocationScore(ctx, request.UserID, request.PartnerID)
	if err != nil {
		log.Printf("Failed to calculate location score: %v", err)
		locationScore = 7 // Default score
	}
	factors.LocationScore = locationScore

	// Quota score (0-20 points)
	quotaScore, err := q.calculateQuotaScore(ctx, request.UserID)
	if err != nil {
		log.Printf("Failed to calculate quota score: %v", err)
		quotaScore = 10 // Default score
	}
	factors.QuotaScore = quotaScore

	// Time in queue score (0-10 points)
	timeInQueueScore := q.calculateTimeInQueueScore(request.CreatedAt)
	factors.TimeInQueueScore = timeInQueueScore

	// Reason score (0-10 points)
	reasonScore := q.calculateReasonScore(request.Reason)
	factors.ReasonScore = reasonScore

	// Calculate total score
	totalScore := factors.UserHistoryScore + factors.UrgencyScore + factors.LocationScore +
		factors.QuotaScore + factors.TimeInQueueScore + factors.ReasonScore

	return totalScore, nil
}

// calculateUserHistoryScore calculates score based on user's past behavior
func (q *QueueService) calculateUserHistoryScore(ctx context.Context, userID uuid.UUID) (float64, error) {
	// Get user's completed requests
	completedKey := q.generateUserCompletedKey(userID)
	var completedCount int64
	err := q.cache.Get(ctx, completedKey, &completedCount)
	if err != nil {
		completedCount = 0
	}

	// Get user's cancelled requests
	cancelledKey := q.generateUserCancelledKey(userID)
	var cancelledCount int64
	err = q.cache.Get(ctx, cancelledKey, &cancelledCount)
	if err != nil {
		cancelledCount = 0
	}

	// Calculate score based on completion rate
	totalRequests := completedCount + cancelledCount
	if totalRequests == 0 {
		return 15, nil // Default score for new users
	}

	completionRate := float64(completedCount) / float64(totalRequests)
	score := completionRate * 25 // Max 25 points

	return score, nil
}

// calculateUrgencyScore calculates score based on pickup date urgency
func (q *QueueService) calculateUrgencyScore(pickupDate *time.Time) float64 {
	if pickupDate == nil {
		return 10 // Default score
	}

	daysUntilPickup := time.Until(*pickupDate).Hours() / 24

	if daysUntilPickup <= 1 {
		return 20 // Very urgent
	} else if daysUntilPickup <= 3 {
		return 15 // Urgent
	} else if daysUntilPickup <= 7 {
		return 10 // Normal
	} else {
		return 5 // Not urgent
	}
}

// calculateLocationScore calculates score based on distance
func (q *QueueService) calculateLocationScore(ctx context.Context, userID, partnerID uuid.UUID) (float64, error) {
	// This would typically involve getting user and partner locations
	// For now, return a default score
	// In real implementation, you would use:
	// - ctx for timeout/cancellation
	// - userID to get user's location
	// - partnerID to get partner's location
	// - Calculate distance and return appropriate score
	_ = ctx       // Suppress unused parameter warning
	_ = userID    // Suppress unused parameter warning
	_ = partnerID // Suppress unused parameter warning
	return 7, nil
}

// calculateQuotaScore calculates score based on remaining quota
func (q *QueueService) calculateQuotaScore(ctx context.Context, userID uuid.UUID) (float64, error) {
	// Get user's remaining quota
	quotaKey := q.generateUserQuotaKey(userID)
	var remainingQuota int
	err := q.cache.Get(ctx, quotaKey, &remainingQuota)
	if err != nil {
		remainingQuota = 3 // Default quota
	}

	// Higher score for users with more remaining quota
	score := float64(remainingQuota) * 6.67 // Max 20 points for 3 quota
	if score > 20 {
		score = 20
	}

	return score, nil
}

// calculateTimeInQueueScore calculates score based on time in queue
func (q *QueueService) calculateTimeInQueueScore(createdAt time.Time) float64 {
	timeInQueue := time.Since(createdAt).Hours()

	if timeInQueue <= 1 {
		return 10 // Max score for recent requests
	} else if timeInQueue <= 24 {
		return 8
	} else if timeInQueue <= 72 {
		return 5
	} else {
		return 2 // Minimum score for old requests
	}
}

// calculateReasonScore calculates score based on reason quality
func (q *QueueService) calculateReasonScore(reason string) float64 {
	if reason == "" {
		return 5 // Default score for no reason
	}

	// Simple scoring based on reason length and keywords
	score := 5.0

	// Bonus for longer, more detailed reasons
	if len(reason) > 50 {
		score += 2
	}
	if len(reason) > 100 {
		score += 2
	}

	// Bonus for certain keywords indicating genuine need
	urgentKeywords := []string{"urgent", "emergency", "interview", "job", "work", "important"}
	for _, keyword := range urgentKeywords {
		if contains(reason, keyword) {
			score += 1
			break
		}
	}

	if score > 10 {
		score = 10
	}

	return score
}

// calculateEstimatedWaitTime calculates estimated wait time based on queue length
func (q *QueueService) calculateEstimatedWaitTime(queueLength int64) time.Duration {
	// Assume average processing time of 2 hours per request
	avgProcessingTime := 2 * time.Hour
	return time.Duration(queueLength) * avgProcessingTime
}

// getQueuePosition gets the current position in queue
func (q *QueueService) getQueuePosition(ctx context.Context, queueKey string) (int, error) {
	queueLength, err := q.cache.LLen(ctx, queueKey)
	if err != nil {
		return 0, err
	}
	return int(queueLength), nil
}

// Helper functions for key generation
func (q *QueueService) generateQueueKey(itemID uuid.UUID, requestType string) string {
	return fmt.Sprintf("queue:%s:%s", requestType, itemID.String())
}

func (q *QueueService) generateRequestKey(requestID uuid.UUID) string {
	return fmt.Sprintf("request:%s", requestID.String())
}

func (q *QueueService) generateUserCompletedKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:completed", userID.String())
}

func (q *QueueService) generateUserCancelledKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:cancelled", userID.String())
}

func (q *QueueService) generateUserQuotaKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:quota", userID.String())
}

// QueueStatus represents the status of a queue
type QueueStatus struct {
	ItemID            uuid.UUID     `json:"item_id"`
	Type              string        `json:"type"`
	QueueLength       int64         `json:"queue_length"`
	EstimatedWaitTime time.Duration `json:"estimated_wait_time"`
	IsOpen            bool          `json:"is_open"`
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			contains(s[1:], substr))))
}
