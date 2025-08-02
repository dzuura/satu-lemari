package user

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/models"
)

// QuotaService handles quota reset operations
type QuotaService struct {
	config     *config.Config
	httpClient *http.Client
}

// NewQuotaService creates a new quota service
func NewQuotaService(cfg *config.Config) *QuotaService {
	return &QuotaService{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// StartDynamicQuotaResetScheduler starts the dynamic quota reset scheduler
// This scheduler checks every hour for users whose quota needs to be reset
func (s *QuotaService) StartDynamicQuotaResetScheduler() {
	log.Printf("Starting dynamic quota reset scheduler (checks every hour)")

	go func() {
		// Create ticker that runs every hour
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run initial check immediately
		s.checkAndResetExpiredQuotas()

		// Then run every hour
		for range ticker.C {
			s.checkAndResetExpiredQuotas()
		}
	}()
}

// checkAndResetExpiredQuotas checks for users whose quota needs reset based on their individual schedule
func (s *QuotaService) checkAndResetExpiredQuotas() {
	log.Printf("Checking for users with expired quotas...")

	now := time.Now()
	today := now.Format("2006-01-02")

	// Get users whose quota_reset_date is today or earlier
	users, err := s.getUsersWithExpiredQuotas(today)
	if err != nil {
		log.Printf("Failed to get users with expired quotas: %v", err)
		return
	}

	if len(users) == 0 {
		log.Printf("No users found with expired quotas")
		return
	}

	log.Printf("Found %d users with expired quotas", len(users))

	// Reset quota for each user
	resetCount := 0
	for _, user := range users {
		log.Printf("Resetting quota for user %s (quota_reset_date: %s)", user.ID, user.QuotaResetDate)
		if err := s.resetUserQuotaDynamic(user.ID, user.CreatedAt); err != nil {
			log.Printf("Failed to reset quota for user %s: %v", user.ID, err)
		} else {
			log.Printf("Successfully reset quota for user %s", user.ID)
			resetCount++
		}
	}

	log.Printf("Successfully reset quota for %d/%d users", resetCount, len(users))
}

// getUsersWithExpiredQuotas gets users whose quota_reset_date is today or earlier
func (s *QuotaService) getUsersWithExpiredQuotas(today string) ([]models.User, error) {
	url := fmt.Sprintf("%s/rest/v1/users?quota_reset_date=lte.%s&role=eq.user&select=id,created_at,quota_reset_date",
		s.config.SupabaseURL, today)

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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get users with expired quotas: %s", string(body))
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}

	return users, nil
}

// resetUserQuotaDynamic resets quota for a user based on their registration date
func (s *QuotaService) resetUserQuotaDynamic(userID string, createdAt time.Time) error {
	now := time.Now()

	// Calculate next reset date based on user's registration date + 7-day cycles
	nextResetDate := s.calculateNextResetDate(createdAt, now)
	nextResetDateStr := nextResetDate.Format("2006-01-02")

	log.Printf("Resetting quota for user %s: next reset on %s (registered on %s, %d days from now)",
		userID, nextResetDateStr, createdAt.Format("2006-01-02"),
		int(nextResetDate.Sub(now).Hours()/24))

	// Update user quota in database
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	updateData := map[string]interface{}{
		"weekly_donation_used": 0,
		"quota_reset_date":     nextResetDateStr,
		"updated_at":           now.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to reset quota: %s", string(body))
	}

	return nil
}

// calculateNextResetDate calculates the next quota reset date based on user's registration date
// Maintains 7-day cycles from registration date
func (s *QuotaService) calculateNextResetDate(createdAt time.Time, now time.Time) time.Time {
	// Calculate days since registration
	daysSinceRegistration := int(now.Sub(createdAt).Hours() / 24)

	// Calculate how many complete 7-day cycles have passed
	completeCycles := daysSinceRegistration / 7

	// Calculate the next reset date (registration + (cycles + 1) * 7 days)
	nextResetDate := createdAt.AddDate(0, 0, (completeCycles+1)*7)

	// If the calculated date is today or in the past, add one more 7-day cycle
	tomorrow := now.AddDate(0, 0, 1)
	if nextResetDate.Before(tomorrow) {
		nextResetDate = nextResetDate.AddDate(0, 0, 7)
	}

	return nextResetDate
}
