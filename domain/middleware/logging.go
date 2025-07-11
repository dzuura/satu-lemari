package middleware

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
	
	"github.com/dzuura/satu-lemari/domain/common"
)

// LoggingMiddleware handles request/response logging
type LoggingMiddleware struct {
	logger *log.Logger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger *log.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: logger,
	}
}

// responseWriter wraps http.ResponseWriter to capture response data
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.body != nil {
		rw.body.Write(b)
	}
	return rw.ResponseWriter.Write(b)
}

// LogRequests logs HTTP requests and responses
func (l *LoggingMiddleware) LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		traceID := common.GenerateTraceID()
		
		// Set trace ID in response header
		w.Header().Set("X-Trace-ID", traceID)
		
		// Read request body (for POST/PUT requests)
		var requestBody []byte
		if r.Body != nil && (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}
		
		// Wrap response writer to capture response
		var responseBody bytes.Buffer
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     200, // default status code
			body:          &responseBody,
		}
		
		// Process request
		next.ServeHTTP(wrapped, r)
		
		// Calculate duration
		duration := time.Since(start)
		
		// Get user info from context if available
		userID, _, role, authenticated := GetUserFromContext(r)
		if !authenticated {
			userID = "anonymous"
			role = "none"
		}
		
		// Log request/response
		l.logger.Printf(
			"[%s] %s %s %d %v | User: %s (%s) | Request: %s | Response: %s",
			traceID,
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
			userID,
			role,
			l.truncateString(string(requestBody), 200),
			l.truncateString(responseBody.String(), 200),
		)
		
		// Log slow requests (> 1 second)
		if duration > time.Second {
			l.logger.Printf("[SLOW REQUEST] %s %s took %v", r.Method, r.URL.Path, duration)
		}
		
		// Log errors (4xx, 5xx status codes)
		if wrapped.statusCode >= 400 {
			l.logger.Printf("[ERROR] %s %s returned %d", r.Method, r.URL.Path, wrapped.statusCode)
		}
	})
}

// LogErrors logs error responses in detail
func (l *LoggingMiddleware) LogErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     200,
			body:          &bytes.Buffer{},
		}
		
		next.ServeHTTP(wrapped, r)
		
		// Log detailed error information for 4xx and 5xx responses
		if wrapped.statusCode >= 400 {
			userID, _, _, _ := GetUserFromContext(r)
			l.logger.Printf(
				"[ERROR DETAIL] Method: %s, Path: %s, Status: %d, User: %s, Response: %s",
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				userID,
				wrapped.body.String(),
			)
		}
	})
}

// truncateString truncates string to specified length
func (l *LoggingMiddleware) truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3]  "..."
}

// RequestIDMiddleware adds unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := common.GenerateTraceID()
		w.Header().Set("X-Request-ID", requestID)
		
		// Add request ID to context
		ctx := r.Context()
		ctx = common.SetRequestID(ctx, requestID)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		
		next.ServeHTTP(w, r)
	})
}