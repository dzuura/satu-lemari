package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dzuura/satu-lemari/domain/common"
	appError "github.com/dzuura/satu-lemari/domain/error"
)

// RateLimiter represents a rate limiter for API requests
type RateLimiter struct {
	requests map[string]*ClientRequests
	mutex    sync.RWMutex
	limit    int
	window   time.Duration
}

// ClientRequests tracks requests for a specific client
type ClientRequests struct {
	count     int
	resetTime time.Time
	mutex     sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*ClientRequests),
		limit:    limit,
		window:   window,
	}

	// Start cleanup goroutine to remove expired entries
	go rl.cleanup()

	return rl
}

// RateLimitMiddleware applies rate limiting to requests
func (rl *RateLimiter) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get client identifier (IP address or user ID if authenticated)
		clientID := rl.getClientID(r)

		// Check if request is allowed
		allowed, resetTime := rl.isAllowed(clientID)

		// Set rate limit headers
		rl.setRateLimitHeaders(w, resetTime)

		if !allowed {
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrRateLimited, "Rate limit exceeded. Please try again later."),
				common.GenerateTraceID())
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientID returns a unique identifier for the client
func (rl *RateLimiter) getClientID(r *http.Request) string {
	// Try to get user ID from context first (for authenticated requests)
	userID, ok := r.Context().Value(common.UserIDKey()).(string)
	if ok && userID != "" {
		return "user:" + userID
	}

	// Fall back to IP address for anonymous requests
	ip := r.RemoteAddr
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ip = forwardedFor
	} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		ip = realIP
	}

	return "ip:" + ip
}

// isAllowed checks if the request is allowed based on rate limit
func (rl *RateLimiter) isAllowed(clientID string) (bool, time.Time) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// Get or create client requests tracker
	clientReqs, exists := rl.requests[clientID]
	if !exists {
		clientReqs = &ClientRequests{
			count:     0,
			resetTime: now.Add(rl.window),
		}
		rl.requests[clientID] = clientReqs
	}

	clientReqs.mutex.Lock()
	defer clientReqs.mutex.Unlock()

	// Reset counter if window has expired
	if now.After(clientReqs.resetTime) {
		clientReqs.count = 0
		clientReqs.resetTime = now.Add(rl.window)
	}

	// Check if limit is exceeded
	if clientReqs.count >= rl.limit {
		return false, clientReqs.resetTime
	}

	// Increment counter and allow request
	clientReqs.count++
	return true, clientReqs.resetTime
}

// setRateLimitHeaders sets rate limit headers in response
func (rl *RateLimiter) setRateLimitHeaders(w http.ResponseWriter, resetTime time.Time) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
	w.Header().Set("X-RateLimit-Window", rl.window.String())
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
}

// cleanup removes expired entries from the rate limiter
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute * 5) // Cleanup every 5 minutes
	defer ticker.Stop()

	for range ticker.C {
		rl.mutex.Lock()
		now := time.Now()

		for clientID, clientReqs := range rl.requests {
			clientReqs.mutex.Lock()
			// Remove entries that are older than the window
			if now.After(clientReqs.resetTime.Add(rl.window)) {
				delete(rl.requests, clientID)
			}
			clientReqs.mutex.Unlock()
		}

		rl.mutex.Unlock()
	}
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	return map[string]interface{}{
		"total_clients": len(rl.requests),
		"limit":         rl.limit,
		"window":        rl.window.String(),
	}
}

// PerEndpointRateLimiter provides different rate limits for different endpoints
type PerEndpointRateLimiter struct {
	limiters map[string]*RateLimiter
	mutex    sync.RWMutex
}

// NewPerEndpointRateLimiter creates a new per-endpoint rate limiter
func NewPerEndpointRateLimiter() *PerEndpointRateLimiter {
	return &PerEndpointRateLimiter{
		limiters: make(map[string]*RateLimiter),
	}
}

// AddEndpoint adds rate limiting for a specific endpoint pattern
func (per *PerEndpointRateLimiter) AddEndpoint(pattern string, limit int, window time.Duration) {
	per.mutex.Lock()
	defer per.mutex.Unlock()

	per.limiters[pattern] = NewRateLimiter(limit, window)
}

// Middleware applies per-endpoint rate limiting
func (per *PerEndpointRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		per.mutex.RLock()

		// Find matching rate limiter for the endpoint
		var rateLimiter *RateLimiter
		for pattern, limiter := range per.limiters {
			if matched := per.matchPattern(r.URL.Path, pattern); matched {
				rateLimiter = limiter
				break
			}
		}

		per.mutex.RUnlock()

		// If no specific rate limiter found, use default or skip
		if rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Apply rate limiting
		rateLimiter.RateLimitMiddleware(next).ServeHTTP(w, r)
	})
}

// matchPattern checks if a path matches a pattern (simple string matching for now)
func (per *PerEndpointRateLimiter) matchPattern(path, pattern string) bool {
	// Simple pattern matching - can be enhanced with regex or more sophisticated matching
	return path == pattern ||
		(len(pattern) > 0 && pattern[len(pattern)-1] == '*' &&
			len(path) >= len(pattern)-1 &&
			path[:len(pattern)-1] == pattern[:len(pattern)-1])
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *PerEndpointRateLimiter {
	per := NewPerEndpointRateLimiter()

	// Authentication endpoints - stricter limits
	per.AddEndpoint("/auth/verify", 10, time.Minute)
	per.AddEndpoint("/auth/login", 5, time.Minute)
	per.AddEndpoint("/auth/register", 3, time.Minute)

	// API endpoints - moderate limits
	per.AddEndpoint("/api/items", 100, time.Minute)
	per.AddEndpoint("/api/requests", 50, time.Minute)
	per.AddEndpoint("/api/users", 30, time.Minute)

	// Upload endpoints - stricter limits due to resource usage
	per.AddEndpoint("/api/upload", 10, time.Minute)

	return per
}

// APIKeyRateLimiter provides rate limiting based on API keys
type APIKeyRateLimiter struct {
	keyLimits map[string]*RateLimiter
	mutex     sync.RWMutex
}

// NewAPIKeyRateLimiter creates a new API key rate limiter
func NewAPIKeyRateLimiter() *APIKeyRateLimiter {
	return &APIKeyRateLimiter{
		keyLimits: make(map[string]*RateLimiter),
	}
}

// SetKeyLimit sets rate limit for a specific API key
func (akrl *APIKeyRateLimiter) SetKeyLimit(apiKey string, limit int, window time.Duration) {
	akrl.mutex.Lock()
	defer akrl.mutex.Unlock()

	akrl.keyLimits[apiKey] = NewRateLimiter(limit, window)
}

// Middleware applies API key based rate limiting
func (akrl *APIKeyRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// No API key provided, continue without specific rate limiting
			next.ServeHTTP(w, r)
			return
		}

		akrl.mutex.RLock()
		rateLimiter, exists := akrl.keyLimits[apiKey]
		akrl.mutex.RUnlock()

		if !exists {
			// Unknown API key, reject request
			appError.WriteErrorResponse(w,
				appError.New(appError.ErrUnauthorized, "Invalid API key"),
				common.GenerateTraceID())
			return
		}

		// Apply rate limiting for this API key
		rateLimiter.RateLimitMiddleware(next).ServeHTTP(w, r)
	})
}
