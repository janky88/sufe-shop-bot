package bot

import (
	"fmt"
	"strings"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	
	"shop-bot/internal/bot/messages"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// handleMyOrders shows user's order history
func (b *Bot) handleMyOrders(message *tgbotapi.Message) {
	// Get user
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Get user's orders
	orders, err := store.GetUserOrders(b.db, user.ID, 10, 0)
	if err != nil {
		logger.Error("Failed to get user orders", "error", err)
		b.sendError(message.Chat.ID, b.msg.Get(lang, "failed_to_load_orders"))
		return
	}
	
	// Build order list message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(b.msg.Get(lang, "my_orders_title"))
	msgBuilder.WriteString("\n\n")
	
	if len(orders) == 0 {
		msgBuilder.WriteString(b.msg.Get(lang, "no_orders_yet"))
	} else {
		for _, order := range orders {
			status := b.msg.Get(lang, "order_status_"+order.Status)
			
			orderInfo := fmt.Sprintf(
				"🆔 #%d | %s\n📦 %s\n💰 $%.2f | 🕐 %s\n\n",
				order.ID,
				status,
				order.Product.Name,
				float64(order.AmountCents)/100,
				order.CreatedAt.Format("01/02 15:04"),
			)
			msgBuilder.WriteString(orderInfo)
		}
	}
	
	// Add inline keyboard for viewing specific order
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.msg.Get(lang, "view_order_details"), "order_list"),
		),
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, msgBuilder.String())
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// handleOrderDetails shows detailed order information
func (b *Bot) handleOrderDetails(callback *tgbotapi.CallbackQuery, orderID uint) {
	// Get user
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)
	
	// Get order with validation that it belongs to user
	order, err := store.GetUserOrder(b.db, user.ID, orderID)
	if err != nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, b.msg.Get(lang, "order_not_found")))
		return
	}
	
	// Build detailed order message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(b.msg.Format(lang, "order_details_title", map[string]interface{}{
		"OrderID": order.ID,
	}))
	msgBuilder.WriteString("\n\n")
	
	// Order information
	msgBuilder.WriteString(b.msg.Format(lang, "order_details", map[string]interface{}{
		"ProductName": order.Product.Name,
		"Price":       fmt.Sprintf("%.2f", float64(order.AmountCents)/100),
		"Status":      b.msg.Get(lang, "order_status_"+order.Status),
		"CreatedAt":   order.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaidAt":      formatTime(order.PaidAt),
		"BalanceUsed": fmt.Sprintf("%.2f", float64(order.BalanceUsed)/100),
		"PaymentAmount": fmt.Sprintf("%.2f", float64(order.PaymentAmount)/100),
	}))
	
	// If order is delivered, show the code again
	if order.Status == "delivered" {
		var code store.Code
		if err := b.db.Where("order_id = ?", order.ID).First(&code).Error; err == nil {
			msgBuilder.WriteString("\n\n")
			msgBuilder.WriteString(b.msg.Format(lang, "order_code_resend", map[string]interface{}{
				"Code": code.Code,
			}))
		}
	}
	
	// Back button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.msg.Get(lang, "back_to_orders"), "my_orders"),
		),
	)
	
	edit := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		msgBuilder.String(),
	)
	edit.ReplyMarkup = &keyboard
	edit.ParseMode = "Markdown"
	
	b.api.Send(edit)
	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
}

// formatTime formats a time pointer
func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}