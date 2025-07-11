package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/category"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/dzuura/satu-lemari/domain/middleware"
	"github.com/dzuura/satu-lemari/domain/user"
)

type Server struct {
	config      *config.Config
	authService *auth.AuthService
	userService *user.UserService
	categoryService *category.CategoryService
	itemService *item.ItemService
	router      *mux.Router
	httpServer  *http.Server
}

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize server
	server := NewServer(cfg)

	// Setup routes and middleware
	server.setupRoutes()

	// Start server
	server.start()
}

func NewServer(cfg *config.Config) *Server {
	// Initialize services
	authService := auth.NewAuthService(cfg)
	userService := user.NewUserService(cfg)
	categoryService := category.NewCategoryService(cfg)
	itemService := item.NewItemService(cfg)

	// Initialize router
	router := mux.NewRouter()

	return &Server{
		config:          cfg,
		authService:     authService,
		userService:     userService,
		categoryService: categoryService,
		itemService:     itemService,
		router:          router,
	}
}

func (s *Server) setupRoutes() {
	// API version prefix
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Setup middleware
	s.setupMiddleware(api)

	// Register service routes
	s.registerRoutes(api)

	// Health check
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")
	
	// API documentation
	s.router.HandleFunc("/", s.welcome).Methods("GET")
}

func (s *Server) setupMiddleware(api *mux.Router) {
	// Global middleware for all API routes
	api.Use(middleware.LoggingMiddleware)
	api.Use(middleware.RequestIDMiddleware)
	api.Use(middleware.SecurityHeadersMiddleware)

	// Rate limiting middleware
	rateLimiter := middleware.NewRateLimiter(
		100, // requests per minute
		1*time.Minute,
	)
	api.Use(rateLimiter.Middleware)

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins: []string{
			"http://localhost:3000",  // Next.js dev
			"http://localhost:3001",  // Alternative dev port
			"https://satu-lemari.vercel.app", // Production web
			// Add your production domains here
		},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
			"X-Request-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-Total-Count",
		},
		AllowCredentials: true,
		MaxAge:           300, // 5 minutes
	})

	api.Use(c.Handler)
}

func (s *Server) registerRoutes(api *mux.Router) {
	// Public routes (no authentication required)
	publicRoutes := api.PathPrefix("").Subrouter()
	
	// Auth routes
	s.authService.RegisterRoutes(publicRoutes)
	
	// Public category routes
	s.categoryService.RegisterRoutes(publicRoutes)
	
	// Public item routes
	s.itemService.RegisterRoutes(publicRoutes)
	
	// Public user routes (search, profiles)
	s.userService.RegisterRoutes(publicRoutes)

	// Protected routes (authentication required)
	protectedRoutes := api.PathPrefix("").Subrouter()
	
	// Authentication middleware
	authMiddleware := middleware.NewAuthenticationMiddleware(s.authService.GetClient())
	protectedRoutes.Use(authMiddleware.Middleware)

	// Protected user routes
	protectedRoutes.HandleFunc("/users/me", s.userService.GetMyProfile).Methods("GET")
	protectedRoutes.HandleFunc("/users/me", s.userService.UpdateMyProfile).Methods("PUT")
	protectedRoutes.HandleFunc("/users/me/location", s.userService.UpdateMyLocation).Methods("PUT")
	protectedRoutes.HandleFunc("/users/dashboard", s.userService.GetDashboard).Methods("GET")

	// Protected item routes
	protectedRoutes.HandleFunc("/items", s.itemService.CreateItem).Methods("POST")
	protectedRoutes.HandleFunc("/items/mine", s.itemService.GetMyItems).Methods("GET")
	protectedRoutes.HandleFunc("/items/{item_id}", s.itemService.UpdateItem).Methods("PUT")
	protectedRoutes.HandleFunc("/items/{item_id}/status", s.itemService.UpdateItemStatus).Methods("PATCH")
	protectedRoutes.HandleFunc("/items/{item_id}", s.itemService.DeleteItem).Methods("DELETE")
	protectedRoutes.HandleFunc("/items/{item_id}/ai-analyze", s.itemService.AnalyzeItemWithAI).Methods("POST")

	// Admin only routes
	adminRoutes := api.PathPrefix("").Subrouter()
	
	// Authentication  Admin authorization middleware
	adminRoutes.Use(authMiddleware.Middleware)
	adminAuthMiddleware := middleware.NewAuthorizationMiddleware([]string{"admin"})
	adminRoutes.Use(adminAuthMiddleware.Middleware)

	// Admin category routes
	adminRoutes.HandleFunc("/categories", s.categoryService.CreateCategory).Methods("POST")
	adminRoutes.HandleFunc("/categories/{category_id}", s.categoryService.UpdateCategory).Methods("PUT")
	adminRoutes.HandleFunc("/categories/{category_id}", s.categoryService.DeleteCategory).Methods("DELETE")

	// Partner only routes
	partnerRoutes := api.PathPrefix("").Subrouter()
	
	// Authentication  Partner authorization middleware
	partnerRoutes.Use(authMiddleware.Middleware)
	partnerAuthMiddleware := middleware.NewAuthorizationMiddleware([]string{"partner", "admin"})
	partnerRoutes.Use(partnerAuthMiddleware.Middleware)

	// Partner routes are already handled in the protected routes section
	// This section is for future partner-specific endpoints
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "satu-lemari-api",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"checks": map[string]string{
			"database": "connected", // You could add actual database health check here
			"firebase": "connected", // You could add actual Firebase health check here
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health check response: %v", err)
	}
}

func (s *Server) welcome(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"message":     "Welcome to SatuLemari API",
		"version":     "1.0.0",
		"description": "Donation and rental clothing platform API",
		"docs":        "/docs",
		"health":      "/health",
		"endpoints": map[string]interface{}{
			"auth":       "/api/v1/auth",
			"users":      "/api/v1/users",
			"categories": "/api/v1/categories",
			"items":      "/api/v1/items",
		},
		"features": []string{
			"Firebase Authentication",
			"Role-based Access Control",
			"AI Item Analysis",
			"Real-time Notifications",
			"Weekly Donation Quotas",
			"Geolocation Services",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding welcome response: %v", err)
	}
}

func (s *Server) start() {
	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         ":"  s.config.Port,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("🚀 SatuLemari API server starting on port %s", s.config.Port)
		log.Printf("📚 Environment: %s", s.config.Environment)
		log.Printf("🔗 Health check: http://localhost:%s/health", s.config.Port)
		log.Printf("📋 API docs: http://localhost:%s/", s.config.Port)
		
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	s.waitForShutdown()
}

func (s *Server) waitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	<-quit
	log.Println("🛑 Shutting down server...")

	// Create a deadline for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Disable keep-alives
	s.httpServer.SetKeepAlivesEnabled(false)

	// Attempt graceful shutdown
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("❌ Server forced to shutdown: %v", err)
	} else {
		log.Println("✅ Server gracefully stopped")
	}
}

// For JSON encoding in health check and welcome endpoints
import "encoding/json"