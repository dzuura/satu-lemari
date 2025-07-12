package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/dzuura/satu-lemari/domain/database"
	"github.com/dzuura/satu-lemari/domain/models"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// UserRepository handles all database operations for users
type UserRepository struct {
	db *database.Database
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *database.Database) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (
			id, email, username, full_name, role, phone, address, city,
			latitude, longitude, photo, description, is_active,
			weekly_donation_quota, weekly_donation_used, quota_reset_date,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`

	_, err := r.db.Exec(ctx, query,
		user.ID, user.Email, user.Username, user.FullName, user.Role, user.Phone,
		user.Address, user.City, user.Latitude, user.Longitude, user.Photo,
		user.Description, user.IsActive, user.WeeklyDonationQuota,
		user.WeeklyDonationUsed, user.QuotaResetDate, user.CreatedAt, user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, username, full_name, role, phone, address, city,
			   latitude, longitude, photo, description, is_active,
			   weekly_donation_quota, weekly_donation_used, quota_reset_date,
			   created_at, updated_at
		FROM users
		WHERE id = $1 AND is_active = true
	`

	var user models.User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Username, &user.FullName, &user.Role,
		&user.Phone, &user.Address, &user.City, &user.Latitude, &user.Longitude,
		&user.Photo, &user.Description, &user.IsActive, &user.WeeklyDonationQuota,
		&user.WeeklyDonationUsed, &user.QuotaResetDate, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, appError.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by ID: %v", err)
	}

	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, username, full_name, role, phone, address, city,
			   latitude, longitude, photo, description, is_active,
			   weekly_donation_quota, weekly_donation_used, quota_reset_date,
			   created_at, updated_at
		FROM users
		WHERE email = $1 AND is_active = true
	`

	var user models.User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Username, &user.FullName, &user.Role,
		&user.Phone, &user.Address, &user.City, &user.Latitude, &user.Longitude,
		&user.Photo, &user.Description, &user.IsActive, &user.WeeklyDonationQuota,
		&user.WeeklyDonationUsed, &user.QuotaResetDate, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, appError.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %v", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by username
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT id, email, username, full_name, role, phone, address, city,
			   latitude, longitude, photo, description, is_active,
			   weekly_donation_quota, weekly_donation_used, quota_reset_date,
			   created_at, updated_at
		FROM users
		WHERE username = $1 AND is_active = true
	`

	var user models.User
	err := r.db.QueryRow(ctx, query, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.FullName, &user.Role,
		&user.Phone, &user.Address, &user.City, &user.Latitude, &user.Longitude,
		&user.Photo, &user.Description, &user.IsActive, &user.WeeklyDonationQuota,
		&user.WeeklyDonationUsed, &user.QuotaResetDate, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, appError.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %v", err)
	}

	return &user, nil
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users SET
			email = $2, username = $3, full_name = $4, role = $5, phone = $6,
			address = $7, city = $8, latitude = $9, longitude = $10, photo = $11,
			description = $12, is_active = $13, weekly_donation_quota = $14,
			weekly_donation_used = $15, quota_reset_date = $16, updated_at = $17
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query,
		user.ID, user.Email, user.Username, user.FullName, user.Role, user.Phone,
		user.Address, user.City, user.Latitude, user.Longitude, user.Photo,
		user.Description, user.IsActive, user.WeeklyDonationQuota,
		user.WeeklyDonationUsed, user.QuotaResetDate, user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return appError.ErrUserNotFound
	}

	return nil
}

// UpdateLocation updates user location
func (r *UserRepository) UpdateLocation(ctx context.Context, userID uuid.UUID, latitude, longitude float64, address, city string) error {
	query := `
		UPDATE users SET
			latitude = $2, longitude = $3, address = $4, city = $5, updated_at = $6
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, latitude, longitude, address, city, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update user location: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return appError.ErrUserNotFound
	}

	return nil
}

// UpdateDonationQuota updates user's donation quota
func (r *UserRepository) UpdateDonationQuota(ctx context.Context, userID uuid.UUID, used int, resetDate time.Time) error {
	query := `
		UPDATE users SET
			weekly_donation_used = $2, quota_reset_date = $3, updated_at = $4
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, used, resetDate, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update donation quota: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return appError.ErrUserNotFound
	}

	return nil
}

// ResetWeeklyQuotas resets weekly donation quotas for all users
func (r *UserRepository) ResetWeeklyQuotas(ctx context.Context) error {
	query := `
		UPDATE users SET
			weekly_donation_used = 0, quota_reset_date = CURRENT_DATE, updated_at = NOW()
		WHERE quota_reset_date <= CURRENT_DATE - INTERVAL '7 days'
		AND role = 'user'
	`

	_, err := r.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to reset weekly quotas: %v", err)
	}

	return nil
}

// GetPartners retrieves all partner users
func (r *UserRepository) GetPartners(ctx context.Context, limit, offset int) ([]models.UserProfile, error) {
	query := `
		SELECT id, username, full_name, photo, city, description, role, created_at
		FROM users
		WHERE role = 'partner' AND is_active = true
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get partners: %v", err)
	}
	defer rows.Close()

	var partners []models.UserProfile
	for rows.Next() {
		var partner models.UserProfile
		err := rows.Scan(
			&partner.ID, &partner.Username, &partner.FullName, &partner.Photo,
			&partner.City, &partner.Description, &partner.Role, &partner.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan partner: %v", err)
		}
		partners = append(partners, partner)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating partners: %v", err)
	}

	return partners, nil
}

// GetUsersNearby retrieves users within a certain radius
func (r *UserRepository) GetUsersNearby(ctx context.Context, latitude, longitude, radiusKm float64, limit int) ([]models.UserWithLocation, error) {
	query := `
		SELECT id, email, username, full_name, role, phone, address, city,
			   latitude, longitude, photo, description, is_active,
			   weekly_donation_quota, weekly_donation_used, quota_reset_date,
			   created_at, updated_at,
			   (6371 * acos(cos(radians($1)) * cos(radians(latitude)) * 
				cos(radians(longitude) - radians($2)) + sin(radians($1)) * 
				sin(radians(latitude)))) AS distance
		FROM users
		WHERE is_active = true
		AND latitude IS NOT NULL AND longitude IS NOT NULL
		AND (6371 * acos(cos(radians($1)) * cos(radians(latitude)) * 
			cos(radians(longitude) - radians($2)) + sin(radians($1)) * 
			sin(radians(latitude)))) <= $3
		ORDER BY distance
		LIMIT $4
	`

	rows, err := r.db.Query(ctx, query, latitude, longitude, radiusKm, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get nearby users: %v", err)
	}
	defer rows.Close()

	var users []models.UserWithLocation
	for rows.Next() {
		var user models.UserWithLocation
		var distance sql.NullFloat64
		err := rows.Scan(
			&user.ID, &user.Email, &user.Username, &user.FullName, &user.Role,
			&user.Phone, &user.Address, &user.City, &user.Latitude, &user.Longitude,
			&user.Photo, &user.Description, &user.IsActive, &user.WeeklyDonationQuota,
			&user.WeeklyDonationUsed, &user.QuotaResetDate, &user.CreatedAt, &user.UpdatedAt,
			&distance,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan nearby user: %v", err)
		}
		if distance.Valid {
			user.Distance = &distance.Float64
		}
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating nearby users: %v", err)
	}

	return users, nil
}

// CheckEmailExists checks if email already exists
func (r *UserRepository) CheckEmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND is_active = true)`
	
	var exists bool
	err := r.db.QueryRow(ctx, query, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %v", err)
	}

	return exists, nil
}

// CheckUsernameExists checks if username already exists
func (r *UserRepository) CheckUsernameExists(ctx context.Context, username string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND is_active = true)`
	
	var exists bool
	err := r.db.QueryRow(ctx, query, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check username existence: %v", err)
	}

	return exists, nil
}

// GetUserStats retrieves user statistics
func (r *UserRepository) GetUserStats(ctx context.Context, userID uuid.UUID) (*models.UserStats, error) {
	// Get total donations
	donationQuery := `
		SELECT COUNT(*) FROM transactions 
		WHERE user_id = $1 AND type = 'donation' AND status = 'completed'
	`
	var totalDonations int
	err := r.db.QueryRow(ctx, donationQuery, userID).Scan(&totalDonations)
	if err != nil {
		return nil, fmt.Errorf("failed to get total donations: %v", err)
	}

	// Get total rentals
	rentalQuery := `
		SELECT COUNT(*) FROM transactions 
		WHERE user_id = $1 AND type = 'rental' AND status = 'completed'
	`
	var totalRentals int
	err = r.db.QueryRow(ctx, rentalQuery, userID).Scan(&totalRentals)
	if err != nil {
		return nil, fmt.Errorf("failed to get total rentals: %v", err)
	}

	// Get active items (for partners)
	activeItemsQuery := `
		SELECT COUNT(*) FROM items 
		WHERE partner_id = $1 AND status = 'active'
	`
	var activeItems int
	err = r.db.QueryRow(ctx, activeItemsQuery, userID).Scan(&activeItems)
	if err != nil {
		return nil, fmt.Errorf("failed to get active items: %v", err)
	}

	// Get pending requests
	pendingRequestsQuery := `
		SELECT COUNT(*) FROM requests 
		WHERE (user_id = $1 OR partner_id = $1) AND status = 'pending'
	`
	var pendingRequests int
	err = r.db.QueryRow(ctx, pendingRequestsQuery, userID).Scan(&pendingRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending requests: %v", err)
	}

	// Get completed requests
	completedRequestsQuery := `
		SELECT COUNT(*) FROM requests 
		WHERE (user_id = $1 OR partner_id = $1) AND status IN ('approved', 'completed')
	`
	var completedRequests int
	err = r.db.QueryRow(ctx, completedRequestsQuery, userID).Scan(&completedRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get completed requests: %v", err)
	}

	// Get user for quota info
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user for quota info: %v", err)
	}

	stats := &models.UserStats{
		TotalDonations:      totalDonations,
		TotalRentals:        totalRentals,
		ActiveItems:         activeItems,
		PendingRequests:     pendingRequests,
		CompletedRequests:   completedRequests,
		WeeklyQuotaUsed:     user.WeeklyDonationUsed,
		WeeklyQuotaRemaining: user.GetRemainingDonationQuota(),
	}

	return stats, nil
} 