package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop-bot/internal/config"
	"shop-bot/internal/errorhandler"
	"shop-bot/internal/httpadmin"
	"shop-bot/internal/models"
	"shop-bot/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Test error handling system
func TestErrorHandling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken: "test-admin-token",
		JWTSecret:  "test-jwt-secret",
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Use error handling middleware
	router.Use(errorhandler.ErrorHandler(logger))
	
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Test 1: Database error handling
	t.Run("DatabaseErrorHandling", func(t *testing.T) {
		// Create a route that triggers a database error
		router.GET("/test/db-error", func(c *gin.Context) {
			err := gorm.ErrRecordNotFound
			errorhandler.HandleError(c, err)
		})
		
		req := httptest.NewRequest("GET", "/test/db-error", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusNotFound, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "not_found", resp.Error)
		assert.NotEmpty(t, resp.Message)
		assert.NotEmpty(t, resp.RequestID)
		assert.NotZero(t, resp.Timestamp)
	})
	
	// Test 2: Validation error handling
	t.Run("ValidationErrorHandling", func(t *testing.T) {
		// Test invalid product creation
		invalidProduct := map[string]interface{}{
			"name": "", // Empty name should trigger validation error
			"price": -100, // Negative price
		}
		body, _ := json.Marshal(invalidProduct)
		
		req := httptest.NewRequest("POST", "/api/products", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusBadRequest, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "validation_error", resp.Error)
		assert.NotEmpty(t, resp.ValidationErrors)
	})
	
	// Test 3: Authentication error handling
	t.Run("AuthenticationErrorHandling", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/settings", nil)
		// No authorization header
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should redirect for HTML requests
		assert.Equal(t, http.StatusFound, w.Code)
		
		// Test with JSON accept header
		req.Header.Set("Accept", "application/json")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "unauthorized", resp.Error)
	})
	
	// Test 4: Rate limit error handling
	t.Run("RateLimitErrorHandling", func(t *testing.T) {
		// Create a rate limited endpoint
		router.GET("/test/rate-limited", func(c *gin.Context) {
			errorhandler.HandleRateLimitError(c, "Too many requests")
		})
		
		req := httptest.NewRequest("GET", "/test/rate-limited", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.NotEmpty(t, w.Header().Get("Retry-After"))
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "rate_limit_exceeded", resp.Error)
	})
	
	// Test 5: Internal server error handling
	t.Run("InternalServerErrorHandling", func(t *testing.T) {
		// Create endpoint that triggers internal error
		router.GET("/test/internal-error", func(c *gin.Context) {
			err := errors.New("unexpected server error")
			errorhandler.HandleError(c, err)
		})
		
		req := httptest.NewRequest("GET", "/test/internal-error", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "internal_error", resp.Error)
		// Should not expose internal details
		assert.NotContains(t, resp.Message, "unexpected server error")
		assert.Equal(t, "Internal server error", resp.Message)
	})
	
	// Test 6: Custom business logic error
	t.Run("BusinessLogicErrorHandling", func(t *testing.T) {
		// Create endpoint with business logic error
		router.GET("/test/business-error", func(c *gin.Context) {
			err := errorhandler.NewBusinessError("insufficient_balance", "Insufficient balance for this operation")
			errorhandler.HandleError(c, err)
		})
		
		req := httptest.NewRequest("GET", "/test/business-error", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusBadRequest, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "insufficient_balance", resp.Error)
		assert.Equal(t, "Insufficient balance for this operation", resp.Message)
	})
	
	// Test 7: Error recovery in middleware
	t.Run("ErrorRecoveryMiddleware", func(t *testing.T) {
		// Create endpoint that panics
		router.GET("/test/panic", func(c *gin.Context) {
			panic("test panic")
		})
		
		req := httptest.NewRequest("GET", "/test/panic", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		// Should not panic the test
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		
		var resp errorhandler.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.Equal(t, "internal_error", resp.Error)
	})
	
	// Test 8: Content negotiation for errors
	t.Run("ErrorContentNegotiation", func(t *testing.T) {
		// Test JSON response
		req := httptest.NewRequest("GET", "/test/not-found", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
		
		// Test HTML response
		req = httptest.NewRequest("GET", "/test/not-found", nil)
		req.Header.Set("Accept", "text/html")
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, w.Body.String(), "404")
	})
}

// Test error logging
func TestErrorLogging(t *testing.T) {
	// Create logger with custom core to capture logs
	var logEntries []string
	customCore := zap.NewExample().Core()
	logger := zap.New(customCore)
	
	cfg := &config.Config{
		AdminToken: "test-admin-token",
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(errorhandler.ErrorHandler(logger))
	
	// Create endpoint that logs errors
	router.GET("/test/logged-error", func(c *gin.Context) {
		err := errors.New("this error should be logged")
		errorhandler.LogAndHandleError(c, err, logger)
	})
	
	req := httptest.NewRequest("GET", "/test/logged-error", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Verify error was handled
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// Test error metrics
func TestErrorMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken: "test-admin-token",
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Setup metrics
	errorhandler.SetupErrorMetrics()
	router.Use(errorhandler.ErrorHandler(logger))
	router.Use(errorhandler.MetricsMiddleware())
	
	// Create various error endpoints
	router.GET("/test/400", func(c *gin.Context) {
		c.JSON(400, gin.H{"error": "bad request"})
	})
	
	router.GET("/test/500", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "internal error"})
	})
	
	// Make requests to generate metrics
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test/400", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test/500", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	
	// Check metrics endpoint
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Verify metrics are exposed
	body := w.Body.String()
	assert.Contains(t, body, "http_requests_total")
	assert.Contains(t, body, "http_request_duration_seconds")
	assert.Contains(t, body, "http_errors_total")
}

// Test graceful error degradation
func TestGracefulDegradation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken: "test-admin-token",
	}
	
	// Simulate database connection failure
	db := storage.InitTestDB()
	db.Exec("DROP TABLE products") // Break the database
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(errorhandler.ErrorHandler(logger))
	
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Try to access products (should fail gracefully)
	req := httptest.NewRequest("GET", "/api/products", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// Should return error response, not panic
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	var resp errorhandler.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	
	assert.NotEmpty(t, resp.Error)
	assert.NotEmpty(t, resp.RequestID)
}