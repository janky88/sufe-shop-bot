package notification

import (
	"fmt"
	"strings"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	"shop-bot/internal/config"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// EventType represents the type of notification event
type EventType string

const (
	EventNewOrder       EventType = "new_order"
	EventOrderPaid      EventType = "order_paid"
	EventNoStock        EventType = "no_stock"
	EventDeposit        EventType = "deposit"
	EventRechargeUsed   EventType = "recharge_used"
	EventLowStock       EventType = "low_stock"
	EventNewUser        EventType = "new_user"
)

// Service handles admin notifications
type Service struct {
	bot      *tgbotapi.BotAPI
	config   *config.Config
	db       *gorm.DB
	queue    Queue
	channels map[string]Channel
}

// NewService creates a new notification service
func NewService(bot *tgbotapi.BotAPI, config *config.Config, db *gorm.DB) *Service {
	service := &Service{
		bot:      bot,
		config:   config,
		db:       db,
		channels: make(map[string]Channel),
	}
	
	// Register Telegram channel
	if bot != nil {
		telegramChannel := NewTelegramChannel(bot, config)
		service.channels["telegram"] = telegramChannel
	}
	
	// Initialize queue if async notifications are enabled
	if config.AdminNotifications {
		notifConfig := &NotificationConfig{
			Enabled:         true,
			MaxRetries:      3,
			RetryDelay:      time.Second * 2,
			RateLimit:       30, // 30 notifications per minute
			RateLimitWindow: time.Minute,
			AdminChatIDs:    config.AdminChatIDs,
		}
		queue := NewMemoryQueue(service, notifConfig)
		queue.Process() // Start processing queue
		service.queue = queue
	}
	
	return service
}

// NotifyAdmins sends a notification to all configured admin users
func (s *Service) NotifyAdmins(eventType EventType, data map[string]interface{}) {
	// Check if notifications are enabled
	if !s.config.AdminNotifications {
		return
	}
	
	// If queue is available, use async notification
	if s.queue != nil {
		s.NotifyAdminsAsync(eventType, data, PriorityMedium)
		return
	}
	
	// Otherwise send synchronously (legacy behavior)
	s.sendNotification(eventType, data)
}

// NotifyAdminsAsync sends a notification asynchronously with priority
func (s *Service) NotifyAdminsAsync(eventType EventType, data map[string]interface{}, priority Priority) {
	if s.queue == nil {
		logger.Warn("Queue not initialized, falling back to sync notification")
		s.sendNotification(eventType, data)
		return
	}
	
	notification := &Notification{
		Type:     eventType,
		Priority: priority,
		Data:     data,
	}
	
	if err := s.queue.Push(notification); err != nil {
		logger.Error("Failed to queue notification", "error", err)
		// Fallback to sync sending
		s.sendNotification(eventType, data)
	}
}

// sendNotification sends the actual notification (extracted for reuse)
func (s *Service) sendNotification(eventType EventType, data map[string]interface{}) {
	
	// Get admin IDs
	adminIDs := s.config.GetAdminTelegramIDs()
	if len(adminIDs) == 0 {
		return
	}
	
	// Build message based on event type
	message := s.buildMessage(eventType, data)
	if message == "" {
		return
	}
	
	// Send to each admin
	for _, adminID := range adminIDs {
		msg := tgbotapi.NewMessage(adminID, message)
		msg.ParseMode = "Markdown"
		
		if _, err := s.bot.Send(msg); err != nil {
			logger.Error("Failed to send admin notification",
				"admin_id", adminID,
				"event", eventType,
				"error", err,
			)
		}
	}
}

// buildMessage creates a message based on the event type and data
func (s *Service) buildMessage(eventType EventType, data map[string]interface{}) string {
	switch eventType {
	case EventNewOrder:
		return s.buildNewOrderMessage(data)
	case EventOrderPaid:
		return s.buildOrderPaidMessage(data)
	case EventNoStock:
		return s.buildNoStockMessage(data)
	case EventDeposit:
		return s.buildDepositMessage(data)
	case EventRechargeUsed:
		return s.buildRechargeUsedMessage(data)
	case EventLowStock:
		return s.buildLowStockMessage(data)
	case EventNewUser:
		return s.buildNewUserMessage(data)
	default:
		return ""
	}
}

// buildNewOrderMessage creates message for new order event
func (s *Service) buildNewOrderMessage(data map[string]interface{}) string {
	orderID, _ := data["order_id"].(uint)
	userID, _ := data["user_id"].(uint)
	productName, _ := data["product_name"].(string)
	amount, _ := data["amount"].(int)
	
	var user store.User
	if err := s.db.First(&user, userID).Error; err == nil {
		username := getUserDisplayName(&user)
		return fmt.Sprintf(
			"🛒 *新订单*\n\n"+
				"订单号: #%d\n"+
				"用户: %s (ID: %d)\n"+
				"商品: %s\n"+
				"金额: %.2f %s\n"+
				"时间: %s",
			orderID,
			escapeMarkdown(username), userID,
			escapeMarkdown(productName),
			float64(amount)/100, s.config.CurrencySymbol,
			time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	
	return ""
}

// buildOrderPaidMessage creates message for order paid event
func (s *Service) buildOrderPaidMessage(data map[string]interface{}) string {
	orderID, _ := data["order_id"].(uint)
	userID, _ := data["user_id"].(uint)
	productName, _ := data["product_name"].(string)
	amount, _ := data["amount"].(int)
	paymentMethod, _ := data["payment_method"].(string)
	
	var user store.User
	if err := s.db.First(&user, userID).Error; err == nil {
		username := getUserDisplayName(&user)
		return fmt.Sprintf(
			"💰 *订单支付成功*\n\n"+
				"订单号: #%d\n"+
				"用户: %s (ID: %d)\n"+
				"商品: %s\n"+
				"金额: %.2f %s\n"+
				"支付方式: %s\n"+
				"时间: %s",
			orderID,
			escapeMarkdown(username), userID,
			escapeMarkdown(productName),
			float64(amount)/100, s.config.CurrencySymbol,
			paymentMethod,
			time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	
	return ""
}

// buildNoStockMessage creates message for no stock event
func (s *Service) buildNoStockMessage(data map[string]interface{}) string {
	orderID, _ := data["order_id"].(uint)
	productID, _ := data["product_id"].(uint)
	productName, _ := data["product_name"].(string)
	
	// Get current stock count
	var stockCount int64
	s.db.Model(&store.Code{}).Where("product_id = ? AND status = 'available'", productID).Count(&stockCount)
	
	return fmt.Sprintf(
		"⚠️ *商品缺货警告*\n\n"+
			"订单号: #%d\n"+
			"商品: %s (ID: %d)\n"+
			"当前库存: %d\n"+
			"状态: 已支付但无货\n\n"+
			"请及时补充库存！",
		orderID,
		escapeMarkdown(productName), productID,
		stockCount,
	)
}

// buildDepositMessage creates message for deposit event
func (s *Service) buildDepositMessage(data map[string]interface{}) string {
	userID, _ := data["user_id"].(uint)
	amount, _ := data["amount"].(int)
	newBalance, _ := data["new_balance"].(int)
	
	var user store.User
	if err := s.db.First(&user, userID).Error; err == nil {
		username := getUserDisplayName(&user)
		return fmt.Sprintf(
			"💵 *用户充值*\n\n"+
				"用户: %s (ID: %d)\n"+
				"充值金额: %.2f %s\n"+
				"当前余额: %.2f %s\n"+
				"时间: %s",
			escapeMarkdown(username), userID,
			float64(amount)/100, s.config.CurrencySymbol,
			float64(newBalance)/100, s.config.CurrencySymbol,
			time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	
	return ""
}

// buildRechargeUsedMessage creates message for recharge card used event
func (s *Service) buildRechargeUsedMessage(data map[string]interface{}) string {
	userID, _ := data["user_id"].(uint)
	cardCode, _ := data["card_code"].(string)
	amount, _ := data["amount"].(int)
	
	var user store.User
	if err := s.db.First(&user, userID).Error; err == nil {
		username := getUserDisplayName(&user)
		return fmt.Sprintf(
			"🎫 *充值卡使用*\n\n"+
				"用户: %s (ID: %d)\n"+
				"卡号: %s\n"+
				"面额: %.2f %s\n"+
				"时间: %s",
			escapeMarkdown(username), userID,
			escapeMarkdown(cardCode),
			float64(amount)/100, s.config.CurrencySymbol,
			time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	
	return ""
}

// buildLowStockMessage creates message for low stock warning
func (s *Service) buildLowStockMessage(data map[string]interface{}) string {
	productID, _ := data["product_id"].(uint)
	productName, _ := data["product_name"].(string)
	stockCount, _ := data["stock_count"].(int)
	
	return fmt.Sprintf(
		"📉 *低库存警告*\n\n"+
			"商品: %s (ID: %d)\n"+
			"当前库存: %d\n\n"+
			"库存量较低，请考虑补货。",
		escapeMarkdown(productName), productID,
		stockCount,
	)
}

// buildNewUserMessage creates message for new user registration
func (s *Service) buildNewUserMessage(data map[string]interface{}) string {
	userID, _ := data["user_id"].(uint)
	tgUserID, _ := data["tg_user_id"].(int64)
	username, _ := data["username"].(string)
	
	displayName := username
	if username != "" {
		displayName = "@" + username
	} else {
		displayName = fmt.Sprintf("User %d", tgUserID)
	}
	
	return fmt.Sprintf(
		"👤 *新用户注册*\n\n"+
			"用户: %s\n"+
			"用户ID: %d\n"+
			"Telegram ID: %d\n"+
			"时间: %s",
		escapeMarkdown(displayName),
		userID,
		tgUserID,
		time.Now().Format("2006-01-02 15:04:05"),
	)
}

// Helper functions

func getUserDisplayName(user *store.User) string {
	if user.TgUsername != "" {
		return "@" + user.TgUsername
	}
	if user.TgFirstName != "" || user.TgLastName != "" {
		return strings.TrimSpace(user.TgFirstName + " " + user.TgLastName)
	}
	return fmt.Sprintf("User %d", user.TgUserID)
}

func escapeMarkdown(text string) string {
	// Escape special markdown characters
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}
// Stop gracefully stops the notification service
func (s *Service) Stop() {
	if s.queue != nil {
		s.queue.Stop()
	}
}
