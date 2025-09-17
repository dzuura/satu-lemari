package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"github.com/dzuura/satu-lemari/domain/ai"
	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/category"
	"github.com/dzuura/satu-lemari/domain/chat"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/database"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/dzuura/satu-lemari/domain/logging"
	"github.com/dzuura/satu-lemari/domain/middleware"
	"github.com/dzuura/satu-lemari/domain/notification"
	"github.com/dzuura/satu-lemari/domain/orders"
	"github.com/dzuura/satu-lemari/domain/requests"
	"github.com/dzuura/satu-lemari/domain/security"
	"github.com/dzuura/satu-lemari/domain/user"
)

type Server struct {
	config              *config.Config
	authService         *auth.AuthService
	userService         *user.UserService
	categoryService     *category.CategoryService
	itemService         *item.ItemService
	requestService      *requests.RequestService
	ordersService       *orders.OrdersService
	notificationService *notification.NotificationService
	notificationHandler *notification.NotificationHandler
	aiService           *ai.AIServiceManager
	aiHandler           *ai.AIServiceHandler
	chatService         *chat.ChatService
	chatHandler         *chat.ChatHandler
	db                  *database.Database
	cache               *cache.RedisCache
	router              *mux.Router
	httpServer          *http.Server
	logger              *logging.Logger
	passwordHasher      *security.PasswordHasher
}

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize logging with appropriate level based on debug mode
	logLevel := cfg.LogLevel
	if cfg.Debug && logLevel == "info" {
		logLevel = "debug"
	}

	logging.InitLogger(logLevel, cfg.LogFormat)
	logger := logging.GetLogger()
	logger.Info("Starting SatuLemari backend server", map[string]interface{}{
		"port":      cfg.Port,
		"env":       cfg.Env,
		"debug":     cfg.Debug,
		"logLevel":  logLevel,
		"logFormat": cfg.LogFormat,
	})

	// Initialize server
	server, err := NewServer(cfg)
	if err != nil {
		logger.Fatal("Failed to initialize server", map[string]interface{}{
			"error": err.Error(),
		})
	}
	defer server.cleanup()

	// Setup routes and middleware
	server.setupRoutes()

	// Start server
	server.start()
}

func NewServer(cfg *config.Config) (*Server, error) {
	logger := logging.GetLogger()

	// Initialize database
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("Failed to initialize database", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}
	logger.Info("Database initialized successfully", nil)

	// Initialize Redis cache
	redisCache, err := cache.NewRedisCache(cfg)
	if err != nil {
		logger.Warn("Failed to connect to Redis", map[string]interface{}{
			"error": err.Error(),
		})
		redisCache = nil
	} else {
		logger.Info("Redis cache initialized successfully", nil)
	}

	// Initialize password hasher
	passwordHasher := security.NewPasswordHasher(cfg.BcryptCost)
	logger.Info("Password hasher initialized", map[string]interface{}{
		"bcryptCost": cfg.BcryptCost,
	})

	// Initialize services
	authService := auth.NewAuthService(cfg)
	userService := user.NewUserService(cfg, authService.GetClient(), redisCache)
	categoryService := category.NewCategoryService(cfg, redisCache)
	itemService := item.NewItemService(cfg, redisCache)

	// Initialize quota service and start dynamic scheduler
	quotaService := user.NewQuotaService(cfg)
	quotaService.StartDynamicQuotaResetScheduler()

	// Initialize AI services
	aiService, err := ai.NewAIServiceManager(cfg)
	if err != nil {
		logger.Warn("Failed to initialize AI services", map[string]interface{}{
			"error": err.Error(),
		})
		aiService = nil
	} else {
		logger.Info("AI services initialized successfully", map[string]interface{}{
			"gemini_available": aiService.IsGeminiAvailable(),
		})
	}

	// Initialize AI handler
	var aiHandler *ai.AIServiceHandler
	if aiService != nil {
		aiHandler = ai.NewAIServiceHandler(cfg, aiService, redisCache)
	}

	// Initialize notification service and handler
	notificationService, err := notification.NewNotificationService(cfg, redisCache, db)
	if err != nil {
		logger.Error("Failed to initialize notification service", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}
	logger.Info("Notification service initialized successfully", nil)
	notificationHandler := notification.NewNotificationHandler(cfg, notificationService)

	// Initialize request service with notification service
	requestService := requests.NewRequestService(cfg, notificationService)

	// Initialize orders service
	ordersService := orders.NewOrdersService(cfg, db)

	// Initialize chat service
	chatService := chat.NewChatService(cfg, db, redisCache, aiService, itemService, userService)
	chatHandler := chat.NewChatHandler(chatService)

	// Initialize router
	router := mux.NewRouter()

	logger.Info("All services initialized successfully", nil)

	return &Server{
		config:              cfg,
		authService:         authService,
		userService:         userService,
		categoryService:     categoryService,
		itemService:         itemService,
		requestService:      requestService,
		ordersService:       ordersService,
		notificationService: notificationService,
		notificationHandler: notificationHandler,
		aiService:           aiService,
		aiHandler:           aiHandler,
		chatService:         chatService,
		chatHandler:         chatHandler,
		db:                  db,
		cache:               redisCache,
		router:              router,
		logger:              logger,
		passwordHasher:      passwordHasher,
	}, nil
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
	api.Use(middleware.RequestIDMiddleware)
	api.Use(middleware.SecurityHeadersMiddleware)

	// Rate limiting middleware
	rateLimiter := middleware.NewRateLimiter(
		s.config.RateLimitRequests,
		s.config.RateLimitWindow,
	)
	api.Use(rateLimiter.RateLimitMiddleware)

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins: s.config.AllowedOrigins,
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

	// Auth routes - only verify auth is public
	publicRoutes.HandleFunc("/auth/verify", s.authService.VerifyAuth).Methods("POST")

	// Public category routes (GET only)
	s.categoryService.RegisterPublicRoutes(publicRoutes)

	// Public item routes (GET only)
	publicRoutes.HandleFunc("/items", s.itemService.GetItems).Methods("GET")
	publicRoutes.HandleFunc("/items/search", s.itemService.SearchItems).Methods("GET")
	publicRoutes.HandleFunc("/items/{item_id}", s.itemService.GetItem).Methods("GET")

	// Public user routes (only search and profile viewing)
	publicRoutes.HandleFunc("/users/{user_id}/profile", s.userService.GetUserProfile).Methods("GET")
	publicRoutes.HandleFunc("/users/search", s.userService.SearchUsers).Methods("GET")

	// AI routes (public - no authentication required for basic AI features)
	if s.aiHandler != nil {
		s.registerPublicAIRoutes(publicRoutes)
	}

	// Protected routes (authentication required)
	protectedRoutes := api.PathPrefix("").Subrouter()

	// JWT Authentication middleware
	authMiddleware := middleware.NewAuthMiddleware(s.authService.GetJWTService())
	protectedRoutes.Use(authMiddleware.RequireAuth)

	// Protected auth routes
	protectedRoutes.HandleFunc("/auth/refresh", s.authService.RefreshToken).Methods("POST")
	protectedRoutes.HandleFunc("/auth/logout", s.authService.Logout).Methods("POST")

	// Protected user routes
	protectedRoutes.HandleFunc("/users/me", s.userService.GetMyProfile).Methods("GET")
	protectedRoutes.HandleFunc("/users/me", s.userService.UpdateMyProfile).Methods("PUT")
	protectedRoutes.HandleFunc("/users/me", s.userService.DeleteMyAccount).Methods("DELETE")
	protectedRoutes.HandleFunc("/users/dashboard", s.userService.GetDashboard).Methods("GET")

	// Protected item routes - order matters for routing
	protectedRoutes.HandleFunc("/items", s.itemService.CreateItem).Methods("POST")
	protectedRoutes.HandleFunc("/my-items", s.itemService.GetMyItems).Methods("GET")
	protectedRoutes.HandleFunc("/items/{item_id}/status", s.itemService.UpdateItemStatus).Methods("PATCH")
	protectedRoutes.HandleFunc("/items/{item_id}/ai-analyze", s.itemService.AnalyzeItemWithAI).Methods("POST")
	protectedRoutes.HandleFunc("/items/{item_id}", s.itemService.UpdateItem).Methods("PUT")
	protectedRoutes.HandleFunc("/items/{item_id}", s.itemService.DeleteItem).Methods("DELETE")

	// Protected request routes
	protectedRoutes.HandleFunc("/requests", s.requestService.CreateRequest).Methods("POST")
	// Protected orders routes
	protectedRoutes.HandleFunc("/orders", s.ordersService.CreateOrder).Methods("POST")
	protectedRoutes.HandleFunc("/orders", s.ordersService.ListOrders).Methods("GET")
	protectedRoutes.HandleFunc("/orders/my", s.ordersService.GetMyOrders).Methods("GET")
	protectedRoutes.HandleFunc("/orders/partner", s.ordersService.GetPartnerOrders).Methods("GET")
	protectedRoutes.HandleFunc("/orders/{order_id}", s.ordersService.GetOrder).Methods("GET")
	protectedRoutes.HandleFunc("/orders/{order_id}", s.ordersService.DeleteOrder).Methods("DELETE")
	protectedRoutes.HandleFunc("/requests/my", s.requestService.GetMyRequests).Methods("GET")
	protectedRoutes.HandleFunc("/requests/partner", s.requestService.GetPartnerRequests).Methods("GET")
	protectedRoutes.HandleFunc("/requests/{request_id}", s.requestService.GetRequestByID).Methods("GET")
	protectedRoutes.HandleFunc("/requests/{request_id}", s.requestService.UpdateRequest).Methods("PUT")
	protectedRoutes.HandleFunc("/requests/{request_id}", s.requestService.DeleteRequest).Methods("DELETE")

	// Protected notification routes
	s.notificationHandler.RegisterRoutes(protectedRoutes)

	// Protected AI routes (authentication required)
	if s.aiHandler != nil {
		s.registerProtectedAIRoutes(protectedRoutes)
	}

	// Chat routes
	s.chatHandler.RegisterRoutes(api, authMiddleware)

	// Admin only routes
	adminRoutes := api.PathPrefix("").Subrouter()

	// JWT Authentication + Admin authorization middleware
	adminRoutes.Use(authMiddleware.RequireAuth)
	adminRoutes.Use(authMiddleware.RequireAdmin)

	// Admin category routes
	s.categoryService.RegisterAdminRoutes(adminRoutes)

	// Admin orders routes
	adminRoutes.HandleFunc("/orders", s.ordersService.ListOrders).Methods("GET")
	adminRoutes.HandleFunc("/orders/{order_id}", s.ordersService.GetOrder).Methods("GET")
	adminRoutes.HandleFunc("/orders/{order_id}/verify-payment", s.ordersService.VerifyPayment).Methods("POST")
	adminRoutes.HandleFunc("/orders/expire", s.ordersService.ExpireOrders).Methods("POST")
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check database health
	dbStatus := "connected"
	if err := s.db.HealthCheck(ctx); err != nil {
		dbStatus = "disconnected"
		log.Printf("Database health check failed: %v", err)
	}

	// Check Redis health
	cacheStatus := "connected"
	if s.cache != nil {
		if err := s.cache.Ping(ctx); err != nil {
			cacheStatus = "disconnected"
			log.Printf("Redis health check failed: %v", err)
		}
	} else {
		cacheStatus = "not configured"
	}

	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "satu-lemari-api",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"checks": map[string]string{
			"database": dbStatus,
			"cache":    cacheStatus,
			"firebase": "connected",
		},
	}

	// Add debug information if debug mode is enabled
	if s.config.Debug {
		response["debug"] = map[string]interface{}{
			"environment": s.config.Env,
			"port":        s.config.Port,
			"log_level":   s.config.LogLevel,
			"ai_enabled":  s.config.EnableAIFeatures,
		}
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
		"description": "Donation, rental, and thrifting clothing platform API",
		"docs":        "/docs",
		"health":      "/health",
		"endpoints": map[string]interface{}{
			"auth":       "/api/v1/auth",
			"users":      "/api/v1/users",
			"categories": "/api/v1/categories",
			"items":      "/api/v1/items",
			"requests":   "/api/v1/requests",
		},
		"features": []string{
			"Firebase Authentication",
			"Role-based Access Control",
			"AI Item Analysis",
			"Real-time Notifications",
			"Weekly Donation Quotas",
			"Geolocation Services",
			"Redis Caching",
			"Queue System",
			"Request Management",
			"AI-Powered Chat Assistant",
			"Sustainable Fashion Education",
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
		Addr:         ":" + s.config.Port,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on port %s", s.config.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	s.waitForShutdown()
}

func (s *Server) waitForShutdown() {
	// Create channel to listen for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-quit
	log.Println("Server is shutting down...")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server gracefully
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func (s *Server) cleanup() {
	// Close AI services
	if s.aiService != nil {
		if err := s.aiService.Close(); err != nil {
			log.Printf("Error closing AI services: %v", err)
		}
	}

	// Close database connection
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}

	// Close Redis connection
	if s.cache != nil {
		if err := s.cache.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
		}
	}
}

// registerPublicAIRoutes registers AI routes that don't require authentication
func (s *Server) registerPublicAIRoutes(router *mux.Router) {
	// AI Smart Listing endpoints (public)
	router.HandleFunc("/ai/smart-listing", s.aiHandler.SmartListing).Methods("POST")
	router.HandleFunc("/ai/smart-listing/batch", s.aiHandler.BatchSmartListing).Methods("POST")

	// AI Intent Matching endpoints (public)
	router.HandleFunc("/ai/intent", s.aiHandler.ParseIntent).Methods("POST")
	router.HandleFunc("/ai/suggestions", s.aiHandler.GetSuggestions).Methods("GET")

	// AI Status and info (public)
	router.HandleFunc("/ai/status", s.aiHandler.GetAIStatus).Methods("GET")

	// Legacy AI endpoints (public for backward compatibility)
	router.HandleFunc("/ai/analyze", s.aiHandler.AnalyzeItem).Methods("POST")
	router.HandleFunc("/ai/recommendations", s.aiHandler.GetRecommendations).Methods("POST")

	// Public AI recommendation endpoints (no auth required)
	router.HandleFunc("/ai/recommendations/similar/{id}", s.aiHandler.GetSimilarItems).Methods("GET")
	router.HandleFunc("/ai/recommendations/trending", s.aiHandler.GetTrendingRecommendations).Methods("GET")
}

// registerProtectedAIRoutes registers AI routes that require authentication
func (s *Server) registerProtectedAIRoutes(router *mux.Router) {
	// Enhanced AI Recommendation endpoints (authentication required)
	router.HandleFunc("/ai/recommendations/personalized", s.aiHandler.GetPersonalizedRecommendations).Methods("GET")
}
