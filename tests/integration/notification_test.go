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
	"shop-bot/internal/models"
	"shop-bot/internal/notification"
	"shop-bot/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockTelegramAPI for testing
type MockTelegramAPI struct {
	mu          sync.Mutex
	sentMessages []tgbotapi.MessageConfig
	shouldFail   bool
}

func (m *MockTelegramAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.shouldFail {
		return tgbotapi.Message{}, fmt.Errorf("mock send failed")
	}
	
	if msg, ok := c.(tgbotapi.MessageConfig); ok {
		m.sentMessages = append(m.sentMessages, msg)
	}
	
	return tgbotapi.Message{
		MessageID: len(m.sentMessages),
		Chat: &tgbotapi.Chat{
			ID: 12345,
		},
	}, nil
}

func (m *MockTelegramAPI) GetSentMessages() []tgbotapi.MessageConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]tgbotapi.MessageConfig{}, m.sentMessages...)
}

func (m *MockTelegramAPI) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = nil
	m.shouldFail = false
}

// Test admin notification system
func TestAdminNotification(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminToken:       "test-admin-token",
		JWTSecret:        "test-jwt-secret",
		AdminChatID:      12345,
		NotificationTimeout: 5 * time.Second,
	}
	
	db := storage.InitTestDB()
	store := storage.New(db, logger)
	
	// Create mock Telegram API
	mockBot := &MockTelegramAPI{}
	
	// Create notification service
	notifier := notification.New(mockBot, cfg, logger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httpadmin.New(cfg, store, notifier, logger)
	server.SetupRoutes(router)
	
	// Test 1: Order creation notification
	t.Run("OrderCreationNotification", func(t *testing.T) {
		mockBot.Reset()
		
		// Create a test order
		order := &models.Order{
			UserID:      123,
			Username:    "testuser",
			ProductName: "Test Product",
			Quantity:    2,
			TotalPrice:  100.0,
			Status:      "pending",
		}
		
		// Simulate order creation through API
		orderData, _ := json.Marshal(order)
		req := httptest.NewRequest("POST", "/api/orders", bytes.NewReader(orderData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		w := httptest.NewRecorder()
		
		router.ServeHTTP(w, req)
		
		// Wait for async notification
		time.Sleep(100 * time.Millisecond)
		
		// Check notification was sent
		messages := mockBot.GetSentMessages()
		require.Len(t, messages, 1)
		
		msg := messages[0]
		assert.Equal(t, cfg.AdminChatID, msg.ChatID)
		assert.Contains(t, msg.Text, "New Order")
		assert.Contains(t, msg.Text, "testuser")
		assert.Contains(t, msg.Text, "Test Product")
		assert.Contains(t, msg.Text, "100")
	})
	
	// Test 2: Low stock notification
	t.Run("LowStockNotification", func(t *testing.T) {
		mockBot.Reset()
		
		// Create product with low stock
		product := &models.Product{
			Name:        "Low Stock Product",
			Description: "Test product",
			Price:       50.0,
			Stock:       2, // Low stock
		}
		
		err := store.CreateProduct(product)
		require.NoError(t, err)
		
		// Update stock to trigger notification
		err = store.UpdateProductStock(product.ID, 1)
		require.NoError(t, err)
		
		// Trigger low stock check
		notifier.CheckLowStock(store, 5) // Threshold of 5
		
		// Wait for async notification
		time.Sleep(100 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.Len(t, messages, 1)
		
		msg := messages[0]
		assert.Contains(t, msg.Text, "Low Stock Alert")
		assert.Contains(t, msg.Text, "Low Stock Product")
		assert.Contains(t, msg.Text, "1 remaining")
	})
	
	// Test 3: Failed login attempt notification
	t.Run("FailedLoginNotification", func(t *testing.T) {
		mockBot.Reset()
		
		// Make multiple failed login attempts
		for i := 0; i < 3; i++ {
			loginReq := map[string]string{"token": "wrong-token"}
			body, _ := json.Marshal(loginReq)
			
			req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", "192.168.1.100")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
		}
		
		// Wait for async notification
		time.Sleep(100 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.GreaterOrEqual(t, len(messages), 1)
		
		// Find security alert
		found := false
		for _, msg := range messages {
			if strings.Contains(msg.Text, "Security Alert") {
				found = true
				assert.Contains(t, msg.Text, "Multiple failed login attempts")
				assert.Contains(t, msg.Text, "192.168.1.100")
				break
			}
		}
		assert.True(t, found, "Security alert notification should be sent")
	})
	
	// Test 4: System error notification
	t.Run("SystemErrorNotification", func(t *testing.T) {
		mockBot.Reset()
		
		// Trigger a system error
		notifier.NotifyError("Database connection lost", fmt.Errorf("connection refused"))
		
		// Wait for async notification
		time.Sleep(100 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.Len(t, messages, 1)
		
		msg := messages[0]
		assert.Contains(t, msg.Text, "System Error")
		assert.Contains(t, msg.Text, "Database connection lost")
		assert.Contains(t, msg.Text, "connection refused")
	})
	
	// Test 5: Batch notification
	t.Run("BatchNotification", func(t *testing.T) {
		mockBot.Reset()
		
		// Send multiple notifications quickly
		for i := 0; i < 5; i++ {
			notifier.NotifyNewOrder(&models.Order{
				ID:          uint(i + 1),
				Username:    fmt.Sprintf("user%d", i),
				ProductName: fmt.Sprintf("Product %d", i),
				TotalPrice:  float64(i * 10),
			})
		}
		
		// Wait for batch processing
		time.Sleep(500 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		
		// Should batch similar notifications
		assert.LessOrEqual(t, len(messages), 3, "Similar notifications should be batched")
	})
	
	// Test 6: Notification failure handling
	t.Run("NotificationFailureHandling", func(t *testing.T) {
		mockBot.Reset()
		mockBot.shouldFail = true
		
		// Try to send notification
		notifier.NotifyNewOrder(&models.Order{
			ID:          999,
			Username:    "testuser",
			ProductName: "Test Product",
			TotalPrice:  100.0,
		})
		
		// Wait for async processing
		time.Sleep(100 * time.Millisecond)
		
		// Should not crash, error should be logged
		// In real implementation, check logs for error
	})
	
	// Test 7: Notification priority
	t.Run("NotificationPriority", func(t *testing.T) {
		mockBot.Reset()
		
		// Send notifications with different priorities
		notifier.NotifySecurityAlert("Unauthorized access attempt", "192.168.1.1")
		notifier.NotifyNewOrder(&models.Order{
			ID:          1,
			Username:    "user1",
			ProductName: "Product 1",
		})
		notifier.NotifySystemStatus("Daily backup completed")
		
		// Wait for processing
		time.Sleep(200 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.GreaterOrEqual(t, len(messages), 3)
		
		// Security alert should be first (highest priority)
		assert.Contains(t, messages[0].Text, "Security Alert")
	})
	
	// Test 8: Notification formatting
	t.Run("NotificationFormatting", func(t *testing.T) {
		mockBot.Reset()
		
		// Test various notification types for proper formatting
		notifier.NotifyNewOrder(&models.Order{
			ID:          123,
			Username:    "john_doe",
			ProductName: "Premium Package",
			Quantity:    1,
			TotalPrice:  99.99,
			Status:      "completed",
			CreatedAt:   time.Now(),
		})
		
		time.Sleep(100 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.Len(t, messages, 1)
		
		msg := messages[0]
		// Check for proper Markdown formatting
		assert.Contains(t, msg.Text, "🛍")  // Emoji
		assert.Contains(t, msg.Text, "*")   // Bold markers
		assert.Contains(t, msg.Text, "`")   // Code markers
		assert.Equal(t, "Markdown", msg.ParseMode)
	})
}

// Test notification queue management
func TestNotificationQueue(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminChatID:      12345,
		MaxQueueSize:     10,
		NotificationTimeout: 1 * time.Second,
	}
	
	mockBot := &MockTelegramAPI{}
	notifier := notification.New(mockBot, cfg, logger)
	
	// Test queue overflow handling
	t.Run("QueueOverflow", func(t *testing.T) {
		mockBot.Reset()
		mockBot.shouldFail = true // Make sending slow
		
		// Send more notifications than queue size
		for i := 0; i < 15; i++ {
			notifier.NotifyNewOrder(&models.Order{
				ID:       uint(i),
				Username: fmt.Sprintf("user%d", i),
			})
		}
		
		// Wait for processing
		time.Sleep(100 * time.Millisecond)
		
		// Queue should handle overflow gracefully
		stats := notifier.GetQueueStats()
		assert.LessOrEqual(t, stats.QueueLength, cfg.MaxQueueSize)
		assert.Greater(t, stats.DroppedCount, 0)
	})
	
	// Test queue recovery
	t.Run("QueueRecovery", func(t *testing.T) {
		mockBot.Reset()
		mockBot.shouldFail = false // Resume normal operation
		
		// Queue should recover and process pending
		time.Sleep(2 * time.Second)
		
		stats := notifier.GetQueueStats()
		assert.Equal(t, 0, stats.QueueLength)
		assert.Greater(t, stats.ProcessedCount, 0)
	})
}

// Test notification templates
func TestNotificationTemplates(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminChatID: 12345,
	}
	
	mockBot := &MockTelegramAPI{}
	notifier := notification.New(mockBot, cfg, logger)
	
	// Test custom templates
	t.Run("CustomTemplates", func(t *testing.T) {
		mockBot.Reset()
		
		// Set custom template
		template := `
🎉 *New Order Alert*
Customer: {{.Username}}
Product: {{.ProductName}}
Amount: ${{.TotalPrice}}
Time: {{.CreatedAt}}
`
		notifier.SetTemplate("new_order", template)
		
		// Send notification
		notifier.NotifyNewOrder(&models.Order{
			Username:    "alice",
			ProductName: "Special Item",
			TotalPrice:  150.0,
			CreatedAt:   time.Now(),
		})
		
		time.Sleep(100 * time.Millisecond)
		
		messages := mockBot.GetSentMessages()
		require.Len(t, messages, 1)
		
		msg := messages[0]
		assert.Contains(t, msg.Text, "🎉")
		assert.Contains(t, msg.Text, "New Order Alert")
		assert.Contains(t, msg.Text, "alice")
		assert.Contains(t, msg.Text, "Special Item")
		assert.Contains(t, msg.Text, "$150")
	})
}

// Test concurrent notification handling
func TestConcurrentNotifications(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		AdminChatID: 12345,
	}
	
	mockBot := &MockTelegramAPI{}
	notifier := notification.New(mockBot, cfg, logger)
	
	// Send notifications from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < 5; j++ {
				notifier.NotifyNewOrder(&models.Order{
					ID:       uint(id*5 + j),
					Username: fmt.Sprintf("user_%d_%d", id, j),
				})
			}
		}(i)
	}
	
	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	
	// All notifications should be processed
	stats := notifier.GetQueueStats()
	assert.Equal(t, 50, stats.ProcessedCount)
	assert.Equal(t, 0, stats.QueueLength)
}