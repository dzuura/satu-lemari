package main

import (
	"log"
	"net/http"

	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Load configuration
	cfg := config.LoadConfig()

	// Initialize router
	r := mux.NewRouter()

	// Initialize authentication service
	authService := auth.NewAuthService(cfg)

	// Initialize item service
	itemService := item.NewItemService(cfg)

	// Register authentication routes
	authService.RegisterRoutes(r)

	// Register item routes
	itemService.RegisterRoutes(r)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Server is running"))
	}).Methods("GET")

	// Start server
	log.Printf("Server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}
