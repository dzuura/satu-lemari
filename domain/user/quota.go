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

// QuotaService handles weekly quota management
type QuotaService struct {
	config     *config.Config
	httpClient *http.Client
}

// NewQuotaService creates a new quota service instance
func NewQuotaService(cfg *config.Config) *QuotaService {
	return &QuotaService{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StartQuotaResetScheduler starts the weekly quota reset scheduler
func (s *QuotaService) StartQuotaResetScheduler() {
	go func() {
		for {
			now := time.Now()

			// Calculate next Monday at 00:00
			daysUntilMonday := (7 - int(now.Weekday()) + 1) % 7
			if daysUntilMonday == 0 && now.Hour() >= 0 && now.Minute() >= 0 {
				// If it's Monday and past midnight, wait for next Monday
				daysUntilMonday = 7
			}

			nextMonday := now.AddDate(0, 0, daysUntilMonday)
			nextMonday = time.Date(nextMonday.Year(), nextMonday.Month(), nextMonday.Day(), 0, 0, 0, 0, nextMonday.Location())

			// Wait until next Monday
			duration := nextMonday.Sub(now)
			log.Printf("Quota reset scheduler: Next reset in %v (at %v)", duration, nextMonday.Format("2006-01-02 15:04:05"))

			time.Sleep(duration)

			// Reset all user quotas
			if err := s.ResetAllUserQuotas(); err != nil {
				log.Printf("Failed to reset user quotas: %v", err)
			} else {
				log.Printf("Successfully reset all user quotas at %v", time.Now().Format("2006-01-02 15:04:05"))
			}
		}
	}()
}

// ResetAllUserQuotas resets weekly donation quota for all users
func (s *QuotaService) ResetAllUserQuotas() error {
	log.Printf("Starting weekly quota reset for all users...")

	// Get all users
	users, err := s.getAllUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	log.Printf("Found %d users to reset quotas", len(users))

	// Reset quota for each user
	resetCount := 0
	for _, user := range users {
		if err := s.resetUserQuota(user.ID); err != nil {
			log.Printf("Failed to reset quota for user %s: %v", user.ID, err)
		} else {
			resetCount++
		}
	}

	log.Printf("Successfully reset quota for %d/%d users", resetCount, len(users))
	return nil
}

// getAllUsers retrieves all users from database
func (s *QuotaService) getAllUsers() ([]models.User, error) {
	url := fmt.Sprintf("%s/rest/v1/users?select=id,role", s.config.SupabaseURL)

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
		return nil, fmt.Errorf("failed to get users: %s", string(body))
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}

	return users, nil
}

// resetUserQuota resets weekly donation quota for a specific user
func (s *QuotaService) resetUserQuota(userID string) error {
	now := time.Now()
	resetDate := now.Format("2006-01-02")

	// Update user quota in database
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s", s.config.SupabaseURL, userID)
	updateData := map[string]interface{}{
		"weekly_donation_used": 0,
		"quota_reset_date":     resetDate,
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

// CheckAndResetExpiredQuotas checks for users whose quota needs reset based on quota_reset_date
func (s *QuotaService) CheckAndResetExpiredQuotas() error {
	log.Printf("Checking for expired quotas...")

	// Get users whose quota_reset_date is more than 7 days ago
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")

	url := fmt.Sprintf("%s/rest/v1/users?quota_reset_date=lt.%s&select=id,quota_reset_date",
		s.config.SupabaseURL, weekAgo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get expired quotas: %s", string(body))
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return err
	}

	log.Printf("Found %d users with expired quotas", len(users))

	// Reset quota for each expired user
	resetCount := 0
	for _, user := range users {
		if err := s.resetUserQuota(user.ID); err != nil {
			log.Printf("Failed to reset expired quota for user %s: %v", user.ID, err)
		} else {
			resetCount++
		}
	}

	log.Printf("Successfully reset %d expired quotas", resetCount)
	return nil
}

// StartExpiredQuotaChecker starts a periodic checker for expired quotas (runs every hour)
func (s *QuotaService) StartExpiredQuotaChecker() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			if err := s.CheckAndResetExpiredQuotas(); err != nil {
				log.Printf("Error checking expired quotas: %v", err)
			}
		}
	}()
}
