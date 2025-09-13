package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shop-bot/internal/config"
	"shop-bot/internal/httpadmin"
	"shop-bot/internal/security"
	"shop-bot/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Test security features
func TestSecurityFeatures(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:       "test-admin-token",
		JWTSecret:        "test-jwt-secret",
		MaxLoginAttempts: 3,
		LoginWindow:      1 * time.Minute,
		CSRFEnabled:      true,
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Apply security middleware
	router.Use(security.RateLimiter(cfg))
	router.Use(security.CSRFProtection(cfg))
	router.Use(security.SecurityHeaders())
	
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Test 1: XSS Protection
	t.Run("XSSProtection", func(t *testing.T) {
		// Try to inject script tag in product creation
		xssPayload := map[string]interface{}{
			"name":        "<script>alert('XSS')</script>",
			"description": "Test product",
			"price":       10.0,
		}
		body, _ := json.Marshal(xssPayload)
		
		req := httptest.NewRequest("POST", "/api/products", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Product should be created but with sanitized input
		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		
		// Name should be sanitized
		product := resp["product"].(map[string]interface{})
		assert.NotContains(t, product["name"], "<script>")
		assert.NotContains(t, product["name"], "</script>")
	})
	
	// Test 2: SQL Injection Protection
	t.Run("SQLInjectionProtection", func(t *testing.T) {
		// Try SQL injection in search
		req := httptest.NewRequest("GET", "/api/products?search='; DROP TABLE products; --", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should return results or empty, not error
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Verify table still exists
		req = httptest.NewRequest("GET", "/api/products", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	// Test 3: CSRF Protection
	t.Run("CSRFProtection", func(t *testing.T) {
		// Get CSRF token
		req := httptest.NewRequest("GET", "/api/csrf", nil)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var csrfResp map[string]string
		json.Unmarshal(w.Body.Bytes(), &csrfResp)
		csrfToken := csrfResp["token"]
		
		// Try POST without CSRF token
		req = httptest.NewRequest("POST", "/api/products", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should be forbidden without CSRF token
		assert.Equal(t, http.StatusForbidden, w.Code)
		
		// Try with CSRF token
		req = httptest.NewRequest("POST", "/api/products", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		req.Header.Set("X-CSRF-Token", csrfToken)
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should work with CSRF token (will fail on validation, but not CSRF)
		assert.NotEqual(t, http.StatusForbidden, w.Code)
	})
	
	// Test 4: Rate Limiting
	t.Run("RateLimiting", func(t *testing.T) {
		// Make rapid requests
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/api/products", nil)
			req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
			req.Header.Set("X-Forwarded-For", "192.168.1.50")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			
			if i < 5 {
				assert.NotEqual(t, http.StatusTooManyRequests, w.Code)
			} else {
				// Should be rate limited
				assert.Equal(t, http.StatusTooManyRequests, w.Code)
			}
		}
	})
	
	// Test 5: Security Headers
	t.Run("SecurityHeaders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Check security headers
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
		assert.Contains(t, w.Header().Get("Content-Security-Policy"), "default-src")
		assert.NotEmpty(t, w.Header().Get("Strict-Transport-Security"))
	})
	
	// Test 6: Input Validation
	t.Run("InputValidation", func(t *testing.T) {
		// Test various malicious inputs
		tests := []struct {
			name    string
			payload map[string]interface{}
			expect  string
		}{
			{
				name: "negative_price",
				payload: map[string]interface{}{
					"name":  "Product",
					"price": -100,
				},
				expect: "validation_error",
			},
			{
				name: "excessive_quantity",
				payload: map[string]interface{}{
					"product_id": 1,
					"quantity":   999999,
				},
				expect: "validation_error",
			},
			{
				name: "invalid_email",
				payload: map[string]interface{}{
					"email": "not-an-email",
				},
				expect: "validation_error",
			},
		}
		
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				body, _ := json.Marshal(tt.payload)
				req := httptest.NewRequest("POST", "/api/validate", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
				w := httptest.NewRecorder()
				
				router.ServeHTTP(w, req)
				
				assert.Equal(t, http.StatusBadRequest, w.Code)
				
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, tt.expect, resp["error"])
			})
		}
	})
	
	// Test 7: Session Hijacking Protection
	t.Run("SessionHijackingProtection", func(t *testing.T) {
		// Login from one IP
		loginReq := map[string]string{"token": cfg.AdminToken}
		body, _ := json.Marshal(loginReq)
		
		req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "192.168.1.100")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		jwtToken := resp["token"].(string)
		
		// Try to use token from different IP/User-Agent
		req = httptest.NewRequest("GET", "/admin/settings", nil)
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		req.Header.Set("X-Forwarded-For", "10.0.0.1") // Different IP
		req.Header.Set("User-Agent", "Chrome/91.0")  // Different UA
		w = httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Should detect session hijacking attempt
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
	
	// Test 8: Password/Token Storage
	t.Run("SecureTokenStorage", func(t *testing.T) {
		// Verify tokens are hashed in database
		var sessions []struct {
			Token string
		}
		
		db.Raw("SELECT token FROM sessions").Scan(&sessions)
		
		for _, session := range sessions {
			// Token should be hashed, not plaintext
			assert.NotContains(t, session.Token, "test-admin-token")
			assert.NotContains(t, session.Token, ".")  // JWT dots
		}
	})
}

// Test DOS protection
func TestDOSProtection(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken: "test-admin-token",
		MaxRequestSize: 1 * 1024 * 1024, // 1MB
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(security.RequestSizeLimiter(cfg))
	
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Test 1: Large payload protection
	t.Run("LargePayloadProtection", func(t *testing.T) {
		// Create payload larger than limit
		largeData := make([]byte, 2*1024*1024) // 2MB
		
		req := httptest.NewRequest("POST", "/api/products", bytes.NewReader(largeData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
	
	// Test 2: Connection flooding
	t.Run("ConnectionFlooding", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan int, 100)
		
		// Simulate 100 concurrent connections
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				req := httptest.NewRequest("GET", "/api/products", nil)
				req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
				req.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", id))
				w := httptest.NewRecorder()
				
				router.ServeHTTP(w, req)
				results <- w.Code
			}(i)
		}
		
		wg.Wait()
		close(results)
		
		// Count successful vs rate limited
		rateLimited := 0
		for code := range results {
			if code == http.StatusTooManyRequests {
				rateLimited++
			}
		}
		
		// Some requests should be rate limited
		assert.Greater(t, rateLimited, 0)
	})
}

// Test audit logging
func TestAuditLogging(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:    "test-admin-token",
		AuditEnabled:  true,
		AuditLogPath:  "/tmp/audit.log",
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(security.AuditLogger(cfg))
	
	server := httpadmin.New(cfg, store, nil, logger)
	server.SetupRoutes(router)
	
	// Test sensitive operations are logged
	t.Run("SensitiveOperationLogging", func(t *testing.T) {
		operations := []struct {
			method string
			path   string
			body   string
		}{
			{"POST", "/api/login", `{"token":"test-admin-token"}`},
			{"POST", "/api/products", `{"name":"Test","price":10}`},
			{"DELETE", "/api/products/1", ""},
			{"PUT", "/api/settings", `{"key":"value"}`},
		}
		
		for _, op := range operations {
			var req *http.Request
			if op.body != "" {
				req = httptest.NewRequest(op.method, op.path, strings.NewReader(op.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(op.method, op.path, nil)
			}
			
			req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
			req.Header.Set("X-Forwarded-For", "192.168.1.100")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
		}
		
		// Verify audit logs exist
		var auditLogs []models.AuditLog
		db.Find(&auditLogs)
		
		assert.GreaterOrEqual(t, len(auditLogs), 4)
		
		// Check log contents
		for _, log := range auditLogs {
			assert.NotEmpty(t, log.UserID)
			assert.NotEmpty(t, log.Action)
			assert.NotEmpty(t, log.IPAddress)
			assert.NotEmpty(t, log.UserAgent)
			assert.NotZero(t, log.Timestamp)
			
			// Sensitive data should be masked
			if strings.Contains(log.Action, "login") {
				assert.NotContains(t, log.Details, "test-admin-token")
			}
		}
	})
}

// Test encryption
func TestEncryption(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		EncryptionKey: "test-encryption-key-32-bytes-long",
	}
	
	// Test data encryption/decryption
	t.Run("DataEncryption", func(t *testing.T) {
		sensitive := "sensitive-user-data"
		
		encrypted, err := security.Encrypt(sensitive, cfg.EncryptionKey)
		require.NoError(t, err)
		
		// Encrypted should not contain original
		assert.NotContains(t, encrypted, sensitive)
		
		// Should be able to decrypt
		decrypted, err := security.Decrypt(encrypted, cfg.EncryptionKey)
		require.NoError(t, err)
		
		assert.Equal(t, sensitive, decrypted)
	})
	
	// Test field-level encryption in database
	t.Run("FieldLevelEncryption", func(t *testing.T) {
		db := storage.InitTestDB()
		
		// Create user with encrypted fields
		user := &models.User{
			TelegramID: 12345,
			Username:   "testuser",
			Phone:      security.MustEncrypt("+1234567890", cfg.EncryptionKey),
			Email:      security.MustEncrypt("test@example.com", cfg.EncryptionKey),
		}
		
		err := db.Create(user).Error
		require.NoError(t, err)
		
		// Verify encrypted in database
		var rawUser map[string]interface{}
		db.Table("users").Where("id = ?", user.ID).First(&rawUser)
		
		// Phone and email should be encrypted
		assert.NotEqual(t, "+1234567890", rawUser["phone"])
		assert.NotEqual(t, "test@example.com", rawUser["email"])
		
		// Should decrypt when loaded
		var loadedUser models.User
		db.First(&loadedUser, user.ID)
		
		decryptedPhone, _ := security.Decrypt(loadedUser.Phone, cfg.EncryptionKey)
		decryptedEmail, _ := security.Decrypt(loadedUser.Email, cfg.EncryptionKey)
		
		assert.Equal(t, "+1234567890", decryptedPhone)
		assert.Equal(t, "test@example.com", decryptedEmail)
	})
}