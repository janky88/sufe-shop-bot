package notification

import (
	"fmt"
	"log"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	
	"shop-bot/internal/config"
)

// TestNotification tests the notification service
func TestNotification() error {
	// Create a test config
	cfg := &config.Config{
		AdminNotifications: true,
		AdminTelegramIDs: "123456789", // Replace with actual admin ID for testing
		CurrencySymbol: "¥",
	}
	
	// Create a mock bot (would need real bot token for actual testing)
	bot := &tgbotapi.BotAPI{
		Token: "test-token",
		Self: tgbotapi.User{
			ID:       0,
			UserName: "test_bot",
		},
	}
	
	// Create test DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to create test db: %w", err)
	}
	
	// Create notification service
	service := NewService(bot, cfg, db)
	
	// Test different notification types
	log.Println("Testing EventNewOrder notification...")
	service.NotifyAdmins(EventNewOrder, map[string]interface{}{
		"order_id":     1,
		"user_id":      100,
		"product_name": "Test Product",
		"amount":       1000, // 10.00
	})
	
	time.Sleep(1 * time.Second)
	
	log.Println("Testing EventOrderPaid notification...")
	service.NotifyAdmins(EventOrderPaid, map[string]interface{}{
		"order_id":       2,
		"user_id":        101,
		"product_name":   "Test Product 2",
		"amount":         2000, // 20.00
		"payment_method": "Epay",
	})
	
	time.Sleep(1 * time.Second)
	
	log.Println("Testing EventNoStock notification...")
	service.NotifyAdmins(EventNoStock, map[string]interface{}{
		"order_id":     3,
		"product_id":   10,
		"product_name": "Out of Stock Product",
	})
	
	time.Sleep(1 * time.Second)
	
	log.Println("Testing EventDeposit notification...")
	service.NotifyAdmins(EventDeposit, map[string]interface{}{
		"user_id":     102,
		"amount":      5000, // 50.00
		"new_balance": 10000, // 100.00
	})
	
	// Wait for async queue to process
	time.Sleep(2 * time.Second)
	
	// Stop the service
	service.Stop()
	
	log.Println("Notification test completed!")
	return nil
}