package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server Configuration
	Port string
	Env  string

	// Database Configuration (Supabase REST API)
	SupabaseURL            string
	SupabaseKey            string
	SupabaseServiceRoleKey string

	// Firebase Configuration (Authentication + FCM)
	FirebaseProjectID   string
	FirebasePrivateKey  string
	FirebaseClientEmail string

	// Supabase Storage
	SupabaseStorageBucket string
	StorageBaseURL        string

	// Redis Configuration
	RedisURL      string
	RedisPassword string
	RedisDB       int

	// JWT Configuration
	JWTSecret string
	JWTExpiry time.Duration

	// AI Services (Gemini API)
	EnableAIFeatures bool
	GeminiAPIKey     string
	GeminiModel      string

	// Email Service
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   time.Duration

	// Logging
	LogLevel  string
	LogFormat string

	// CORS
	AllowedOrigins []string

	// Security
	BcryptCost int

	// File Upload Configuration
	MaxFileSizeMB    int
	AllowedFileTypes []string

	// Feature Flags
	EnableEmailNotifications bool
	EnablePushNotifications  bool

	// Development/Debug Settings
	Debug bool
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	config := &Config{
		// Server Configuration
		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENVIRONMENT", "development"),

		// Database Configuration (Supabase REST API)
		SupabaseURL:            getEnv("SUPABASE_URL", ""),
		SupabaseKey:            getEnv("SUPABASE_KEY", ""),
		SupabaseServiceRoleKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),

		// Firebase Configuration (Authentication + FCM)
		FirebaseProjectID:   getEnv("FIREBASE_PROJECT_ID", ""),
		FirebasePrivateKey:  getEnv("FIREBASE_PRIVATE_KEY", ""),
		FirebaseClientEmail: getEnv("FIREBASE_CLIENT_EMAIL", ""),

		// Supabase Storage
		SupabaseStorageBucket: getEnv("STORAGE_BUCKET", ""),
		StorageBaseURL:        getEnv("STORAGE_BASE_URL", ""),

		// Redis Configuration
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// JWT Configuration
		JWTSecret: getEnv("JWT_SECRET", "your-secret-key"),
		JWTExpiry: getEnvAsDuration("JWT_EXPIRY", 24*time.Hour),

		// AI Services (Gemini API)
		EnableAIFeatures: getEnvAsBool("ENABLE_AI_FEATURES", true),
		GeminiAPIKey:     getEnv("GEMINI_API_KEY", ""),
		GeminiModel:      getEnv("GEMINI_MODEL", "gemini-1.5-flash"),

		// Email Service
		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnvAsInt("SMTP_PORT", 587),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),

		// Rate Limiting
		RateLimitRequests: getEnvAsInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow:   getEnvAsDuration("RATE_LIMIT_BURST", time.Minute),

		// Logging
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "json"),

		// CORS
		AllowedOrigins: getEnvAsSlice("ALLOWED_ORIGINS", []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"https://satu-lemari.vercel.app",
		}),

		// Security
		BcryptCost: getEnvAsInt("BCRYPT_COST", 12),

		// File Upload Configuration
		MaxFileSizeMB:    getEnvAsInt("MAX_FILE_SIZE_MB", 10),
		AllowedFileTypes: getEnvAsSlice("ALLOWED_FILE_TYPES", []string{"jpg", "jpeg", "png", "webp"}),

		// Feature Flags
		EnableEmailNotifications: getEnvAsBool("ENABLE_EMAIL_NOTIFICATIONS", true),
		EnablePushNotifications:  getEnvAsBool("ENABLE_PUSH_NOTIFICATIONS", true),

		// Development/Debug Settings
		Debug: getEnvAsBool("DEBUG", false),
	}

	// Validate required configuration
	if err := config.validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	return config
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Split by comma and trim spaces
		slice := strings.Split(value, ",")
		for i, v := range slice {
			slice[i] = strings.TrimSpace(v)
		}
		return slice
	}
	return defaultValue
}

func (c *Config) validate() error {
	// Validate required fields for production
	if c.Env == "production" {
		if c.FirebaseProjectID == "" {
			return fmt.Errorf("FIREBASE_PROJECT_ID is required")
		}
		if c.FirebasePrivateKey == "" {
			return fmt.Errorf("FIREBASE_PRIVATE_KEY is required")
		}
		if c.FirebaseClientEmail == "" {
			return fmt.Errorf("FIREBASE_CLIENT_EMAIL is required")
		}
		if c.SupabaseURL == "" {
			return fmt.Errorf("SUPABASE_URL is required")
		}
		if c.SupabaseKey == "" {
			return fmt.Errorf("SUPABASE_KEY is required")
		}
	}

	return nil
}

// IsDevelopment checks if the application is running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction checks if the application is running in production mode
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}
