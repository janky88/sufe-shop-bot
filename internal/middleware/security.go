package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	
	"github.com/gin-gonic/gin"
)

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	// Rate limiting
	RateLimit          int           // requests per minute
	RateLimitWindow    time.Duration // time window
	RateLimitMessage   string
	
	// CSRF
	EnableCSRF     bool
	CSRFSecret     string
	CSRFCookieName string
	
	// Security headers
	EnableSecurityHeaders bool
	HSTS                 bool
	HSTSMaxAge           int
	ContentTypeNosniff   bool
	XFrameOptions        string // DENY, SAMEORIGIN, or ALLOW-FROM uri
	XSSProtection        bool
	
	// CORS
	EnableCORS       bool
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		RateLimit:             100,
		RateLimitWindow:       time.Minute,
		RateLimitMessage:      "Too many requests. Please try again later.",
		EnableCSRF:            true,
		CSRFSecret:            "", // Will be generated if empty
		CSRFCookieName:        "csrf_token",
		EnableSecurityHeaders: true,
		HSTS:                  true,
		HSTSMaxAge:            31536000, // 1 year
		ContentTypeNosniff:    true,
		XFrameOptions:         "SAMEORIGIN",
		XSSProtection:         true,
		EnableCORS:            false,
		AllowedOrigins:        []string{"*"},
		AllowedMethods:        []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:        []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials:      true,
	}
}

// RateLimiter tracks request rates
type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	
	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		
		for range ticker.C {
			rl.cleanup()
		}
	}()
	
	return rl
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-rl.window)
	
	// Get existing requests
	requests, exists := rl.requests[key]
	if !exists {
		rl.requests[key] = []time.Time{now}
		return true
	}
	
	// Filter out old requests
	var validRequests []time.Time
	for _, t := range requests {
		if t.After(windowStart) {
			validRequests = append(validRequests, t)
		}
	}
	
	// Check limit
	if len(validRequests) >= rl.limit {
		rl.requests[key] = validRequests
		return false
	}
	
	// Add new request
	validRequests = append(validRequests, now)
	rl.requests[key] = validRequests
	
	return true
}

// cleanup removes old entries
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-rl.window)
	
	for key, requests := range rl.requests {
		var validRequests []time.Time
		for _, t := range requests {
			if t.After(windowStart) {
				validRequests = append(validRequests, t)
			}
		}
		
		if len(validRequests) == 0 {
			delete(rl.requests, key)
		} else {
			rl.requests[key] = validRequests
		}
	}
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(limit int, window time.Duration, message string) gin.HandlerFunc {
	if limit <= 0 {
		limit = 100
	}
	if window <= 0 {
		window = time.Minute
	}
	if message == "" {
		message = "Too many requests. Please try again later."
	}
	
	limiter := NewRateLimiter(limit, window)
	
	return func(c *gin.Context) {
		// Get client IP
		clientIP := c.ClientIP()
		
		// Check rate limit
		if !limiter.Allow(clientIP) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": message,
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware(config *SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.EnableSecurityHeaders {
			// HSTS
			if config.HSTS {
				hstsValue := "max-age=" + strconv.Itoa(config.HSTSMaxAge)
				c.Header("Strict-Transport-Security", hstsValue)
			}
			
			// Content-Type Options
			if config.ContentTypeNosniff {
				c.Header("X-Content-Type-Options", "nosniff")
			}
			
			// Frame Options
			if config.XFrameOptions != "" {
				c.Header("X-Frame-Options", config.XFrameOptions)
			}
			
			// XSS Protection
			if config.XSSProtection {
				c.Header("X-XSS-Protection", "1; mode=block")
			}
			
			// Additional security headers
			c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Header("Feature-Policy", "geolocation 'none'; microphone 'none'; camera 'none'")
			c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; font-src 'self' https://cdnjs.cloudflare.com data:; img-src 'self' data: https:; connect-src 'self';")
		}
		
		c.Next()
	}
}

// CSRFToken represents a CSRF token
type CSRFToken struct {
	Token     string
	ExpiresAt time.Time
}

var (
	csrfTokens = make(map[string]*CSRFToken)
	csrfMu     sync.RWMutex
)

// CSRFMiddleware provides CSRF protection
func CSRFMiddleware(secret string, cookieName string) gin.HandlerFunc {
	if secret == "" {
		// Generate a random secret
		secret = generateRandomString(32)
	}
	if cookieName == "" {
		cookieName = "csrf_token"
	}
	
	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			cleanupCSRFTokens()
		}
	}()
	
	return func(c *gin.Context) {
		// Skip CSRF for safe methods and API endpoints
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		
		// Skip for API endpoints with valid JWT
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			if _, exists := c.Get("user_claims"); exists {
				c.Next()
				return
			}
		}
		
		// Get token from request
		var token string
		
		// Check header first
		token = c.GetHeader("X-CSRF-Token")
		if token == "" {
			// Check form value
			token = c.PostForm("csrf_token")
		}
		
		// Validate token
		if token == "" || !validateCSRFToken(token) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Invalid CSRF token",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// GenerateCSRFToken generates a new CSRF token
func GenerateCSRFToken() string {
	token := generateRandomString(32)
	
	csrfMu.Lock()
	csrfTokens[token] = &CSRFToken{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	csrfMu.Unlock()
	
	return token
}

// GetCSRFToken retrieves or generates a CSRF token for a session
func GetCSRFToken(c *gin.Context) string {
	// Try to get existing token from cookie
	if cookie, err := c.Cookie("csrf_token"); err == nil && validateCSRFToken(cookie) {
		return cookie
	}
	
	// Generate new token
	token := GenerateCSRFToken()
	
	// Set cookie
	c.SetCookie("csrf_token", token, 86400, "/", "", false, false)
	
	return token
}

// validateCSRFToken validates a CSRF token
func validateCSRFToken(token string) bool {
	csrfMu.RLock()
	defer csrfMu.RUnlock()
	
	t, exists := csrfTokens[token]
	if !exists {
		return false
	}
	
	if time.Now().After(t.ExpiresAt) {
		return false
	}
	
	return true
}

// cleanupCSRFTokens removes expired CSRF tokens
func cleanupCSRFTokens() {
	csrfMu.Lock()
	defer csrfMu.Unlock()
	
	now := time.Now()
	for token, t := range csrfTokens {
		if now.After(t.ExpiresAt) {
			delete(csrfTokens, token)
		}
	}
}

// CORSMiddleware handles CORS
func CORSMiddleware(config *SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.EnableCORS {
			c.Next()
			return
		}
		
		origin := c.Request.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range config.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}
		
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			
			if config.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
			
			if c.Request.Method == "OPTIONS" {
				// Handle preflight request
				c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				c.Header("Access-Control-Max-Age", "86400")
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		
		c.Next()
	}
}

// generateRandomString generates a random string of given length
func generateRandomString(length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based generation
		return base64.URLEncoding.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	return base64.URLEncoding.EncodeToString(b)[:length]
}