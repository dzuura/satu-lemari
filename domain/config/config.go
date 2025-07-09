package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	FirebaseProjectID string
	FirebasePrivateKey string
	FirebaseClientEmail string
	SupabaseURL     string
	SupabaseKey     string
	RedisURL        string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return &Config{
		Port:            os.Getenv("PORT"),
		FirebaseProjectID: os.Getenv("FIREBASE_PROJECT_ID"),
		FirebasePrivateKey: os.Getenv("FIREBASE_PRIVATE_KEY"),
		FirebaseClientEmail: os.Getenv("FIREBASE_CLIENT_EMAIL"),
		SupabaseURL:     os.Getenv("SUPABASE_URL"),
		SupabaseKey:     os.Getenv("SUPABASE_KEY"),
		RedisURL:        os.Getenv("REDIS_URL"),
	}
}