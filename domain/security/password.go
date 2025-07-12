package security

import (
	"golang.org/x/crypto/bcrypt"
)

// PasswordHasher handles password hashing and verification
type PasswordHasher struct {
	cost int
}

// NewPasswordHasher creates a new password hasher with specified cost
func NewPasswordHasher(cost int) *PasswordHasher {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}

	return &PasswordHasher{
		cost: cost,
	}
}

// HashPassword hashes a password using bcrypt
func (p *PasswordHasher) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), p.cost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// VerifyPassword verifies a password against its hash
func (p *PasswordHasher) VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// GetCost returns the current bcrypt cost
func (p *PasswordHasher) GetCost() int {
	return p.cost
}

// SetCost sets the bcrypt cost (for testing or dynamic adjustment)
func (p *PasswordHasher) SetCost(cost int) {
	if cost >= bcrypt.MinCost && cost <= bcrypt.MaxCost {
		p.cost = cost
	}
}
