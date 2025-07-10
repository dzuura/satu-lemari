package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/dzuura/satu-lemari/domain/queue"
	// "github.com/dzuura/satu-lemari/domain/notification"
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

	// Initialize services
	authService := auth.NewAuthService(cfg)
	itemService := item.NewItemService(cfg)
	queueService := queue.NewQueueService(cfg)
	// notificationService := notification.NewNotificationService(cfg)

	// Register routes
	authService.RegisterRoutes(r)
	itemService.RegisterRoutes(r)
	queueService.RegisterRoutes(r)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Server is running"))
	}).Methods("GET")

	// Start server
	log.Printf("Server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}