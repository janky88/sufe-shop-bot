package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"shop-bot/internal/config"
	"shop-bot/internal/httpadmin"
	"shop-bot/internal/models"
	"shop-bot/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Test JWT authentication flow
func TestJWTAuthentication(t *testing.T) {
	// Setup test environment
	logger, _ := zap.NewDevelopment()
	
	// Create test config
	cfg := &config.Config{
		AdminToken:       "test-admin-token",
		JWTSecret:        "test-jwt-secret-key-for-testing",
		JWTExpireHours:   24,
		RefreshExpireDays: 7,
	}
	
	// Initialize storage
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Test 1: Login with valid legacy token
	t.Run("LoginWithValidLegacyToken", func(t *testing.T) {
		loginReq := map[string]string{"token": cfg.AdminToken}
		body, _ := json.Marshal(loginReq)
		
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["token"])
		assert.NotEmpty(t, resp["refresh_token"])
		
		// Store tokens for next tests
		jwtToken := resp["token"].(string)
		refreshToken := resp["refresh_token"].(string)
		
		// Test 2: Access protected endpoint with JWT
		t.Run("AccessWithJWT", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin/settings", nil)
			req.Header.Set("Authorization", "Bearer "+jwtToken)
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			
			assert.Equal(t, http.StatusOK, w.Code)
		})
		
		// Test 3: Refresh token
		t.Run("RefreshToken", func(t *testing.T) {
			refreshReq := map[string]string{"refresh_token": refreshToken}
			body, _ := json.Marshal(refreshReq)
			
			req := httptest.NewRequest("POST", "/api/refresh", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			
			assert.Equal(t, http.StatusOK, w.Code)
			
			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			
			assert.True(t, resp["success"].(bool))
			assert.NotEmpty(t, resp["token"])
		})
	})
	
	// Test 4: Login with invalid token
	t.Run("LoginWithInvalidToken", func(t *testing.T) {
		loginReq := map[string]string{"token": "invalid-token"}
		body, _ := json.Marshal(loginReq)
		
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		
		assert.False(t, resp["success"].(bool))
	})
	
	// Test 5: Access with invalid JWT
	t.Run("AccessWithInvalidJWT", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/settings", nil)
		req.Header.Set("Authorization", "Bearer invalid-jwt-token")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should redirect to login
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "/admin/login")
	})
	
	// Test 6: Backward compatibility with legacy token
	t.Run("BackwardCompatibilityLegacyToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/settings", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	// Test 7: Expired token handling
	t.Run("ExpiredTokenHandling", func(t *testing.T) {
		// Create an expired token
		expiredTime := time.Now().Add(-25 * time.Hour)
		expiredToken := server.GenerateJWT("admin", expiredTime)
		
		req := httptest.NewRequest("GET", "/admin/settings", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should redirect to login
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "/admin/login")
	})
}

// Test rate limiting on login endpoint
func TestLoginRateLimiting(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:     "test-admin-token",
		JWTSecret:      "test-jwt-secret",
		MaxLoginAttempts: 3,
		LoginWindow:    1 * time.Minute,
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Make multiple failed login attempts
	for i := 0; i < 4; i++ {
		loginReq := map[string]string{"token": "wrong-token"}
		body, _ := json.Marshal(loginReq)
		
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "192.168.1.100")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		if i < 3 {
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		} else {
			// Should be rate limited
			assert.Equal(t, http.StatusTooManyRequests, w.Code)
		}
	}
}

// Test session management
func TestSessionManagement(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:       "test-admin-token",
		JWTSecret:        "test-jwt-secret",
		JWTExpireHours:   1,
		RefreshExpireDays: 7,
		MaxActiveSessions: 2,
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Create multiple sessions
	var sessions []string
	
	for i := 0; i < 3; i++ {
		loginReq := map[string]string{"token": cfg.AdminToken}
		body, _ := json.Marshal(loginReq)
		
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("Test-Browser-%d", i))
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		sessions = append(sessions, resp["token"].(string))
	}
	
	// First session should be invalidated (exceeded max sessions)
	req := httptest.NewRequest("GET", "/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+sessions[0])
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusFound, w.Code) // Should redirect to login
	
	// Latest sessions should still be valid
	for i := 1; i < 3; i++ {
		req := httptest.NewRequest("GET", "/admin/settings", nil)
		req.Header.Set("Authorization", "Bearer "+sessions[i])
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

// Test concurrent token refresh
func TestConcurrentTokenRefresh(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:       "test-admin-token",
		JWTSecret:        "test-jwt-secret",
		JWTExpireHours:   24,
		RefreshExpireDays: 7,
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Login to get refresh token
	loginReq := map[string]string{"token": cfg.AdminToken}
	body, _ := json.Marshal(loginReq)
	
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	var loginResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &loginResp)
	refreshToken := loginResp["refresh_token"].(string)
	
	// Simulate concurrent refresh attempts
	results := make(chan int, 5)
	
	for i := 0; i < 5; i++ {
		go func() {
			refreshReq := map[string]string{"refresh_token": refreshToken}
			body, _ := json.Marshal(refreshReq)
			
			req := httptest.NewRequest("POST", "/api/refresh", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			results <- w.Code
		}()
	}
	
	// Collect results
	successCount := 0
	for i := 0; i < 5; i++ {
		code := <-results
		if code == http.StatusOK {
			successCount++
		}
	}
	
	// Only one refresh should succeed
	assert.Equal(t, 1, successCount, "Only one concurrent refresh should succeed")
}