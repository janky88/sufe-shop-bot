package notification

import (
	"fmt"
	"strings"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/config"
)

// TelegramChannel implements the Channel interface for Telegram notifications
type TelegramChannel struct {
	bot    *tgbotapi.BotAPI
	config *config.Config
}

// NewTelegramChannel creates a new Telegram notification channel
func NewTelegramChannel(bot *tgbotapi.BotAPI, config *config.Config) *TelegramChannel {
	return &TelegramChannel{
		bot:    bot,
		config: config,
	}
}

// Send sends a notification via Telegram
func (t *TelegramChannel) Send(notification *Notification) error {
	if t.bot == nil {
		return fmt.Errorf("telegram bot not initialized")
	}
	
	// Get message based on notification type
	message := t.formatMessage(notification)
	if message == "" {
		return fmt.Errorf("empty message for notification type: %s", notification.Type)
	}
	
	// Send to all admin chat IDs
	adminIDs := t.config.GetAdminTelegramIDs()
	if len(adminIDs) == 0 {
		return fmt.Errorf("no admin telegram IDs configured")
	}
	
	var lastError error
	successCount := 0
	
	for _, adminID := range adminIDs {
		msg := tgbotapi.NewMessage(adminID, message)
		msg.ParseMode = "MarkdownV2"
		
		if _, err := t.bot.Send(msg); err != nil {
			logger.Error("Failed to send notification to admin",
				"admin_id", adminID,
				"error", err)
			lastError = err
		} else {
			successCount++
			logger.Info("Notification sent successfully",
				"admin_id", adminID,
				"type", notification.Type)
		}
	}
	
	// Return error only if all sends failed
	if successCount == 0 && lastError != nil {
		return fmt.Errorf("failed to send to any admin: %w", lastError)
	}
	
	return nil
}

// Name returns the channel name
func (t *TelegramChannel) Name() string {
	return "telegram"
}

// IsEnabled returns whether the channel is enabled
func (t *TelegramChannel) IsEnabled() bool {
	return t.config.AdminNotifications && t.bot != nil
}

// formatMessage formats the notification message based on type
func (t *TelegramChannel) formatMessage(notification *Notification) string {
	// Get the service instance to reuse existing formatters
	service := &Service{
		bot:    t.bot,
		config: t.config,
		db:     nil, // We don't need DB for formatting
	}
	
	// Reuse the existing build methods from service.go
	switch notification.Type {
	case EventNewOrder:
		return service.buildNewOrderMessage(notification.Data)
	case EventOrderPaid:
		return service.buildOrderPaidMessage(notification.Data)
	case EventNoStock:
		return service.buildNoStockMessage(notification.Data)
	case EventDeposit:
		return service.buildDepositMessage(notification.Data)
	case EventRechargeUsed:
		return service.buildRechargeUsedMessage(notification.Data)
	case EventLowStock:
		return service.buildLowStockMessage(notification.Data)
	case EventNewUser:
		return service.buildNewUserMessage(notification.Data)
	default:
		// Generic message format
		text := fmt.Sprintf("🔔 *通知*\n\n类型: `%s`\n", notification.Type)
		for k, v := range notification.Data {
			text += fmt.Sprintf("%s: %v\n", k, v)
		}
		return escapeMarkdownV2(text)
	}
}

// escapeMarkdownV2 escapes special characters for MarkdownV2
func escapeMarkdownV2(text string) string {
	// MarkdownV2 requires escaping these characters
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