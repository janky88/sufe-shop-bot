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

// handleMyOrders shows user's paid order history with pagination
func (b *Bot) handleMyOrders(message *tgbotapi.Message) {
	b.handleMyOrdersPage(message, 0)
}

// handleMyOrdersPage shows a specific page of user's paid orders
func (b *Bot) handleMyOrdersPage(message *tgbotapi.Message, page int) {
	logger.Info("handleMyOrdersPage called", "from", message.From.ID, "page", page)
	
	// Get user
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	// Constants for pagination
	const ordersPerPage = 5
	offset := page * ordersPerPage
	
	// Get total count of paid orders
	totalCount, err := store.GetUserPaidOrderCount(b.db, user.ID)
	if err != nil {
		logger.Error("Failed to get paid order count", "error", err)
		b.sendError(message.Chat.ID, b.msg.Get(lang, "failed_to_load_orders"))
		return
	}
	
	// Get user's paid orders for current page
	orders, err := store.GetUserPaidOrders(b.db, user.ID, ordersPerPage, offset)
	if err != nil {
		logger.Error("Failed to get user paid orders", "error", err)
		b.sendError(message.Chat.ID, b.msg.Get(lang, "failed_to_load_orders"))
		return
	}
	
	// Build order list message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(b.msg.Get(lang, "my_orders_title"))
	msgBuilder.WriteString("\n\n")
	
	if totalCount == 0 {
		msgBuilder.WriteString(b.msg.Get(lang, "no_orders_yet"))
	} else {
		// Show page info
		totalPages := int((totalCount + ordersPerPage - 1) / ordersPerPage)
		msgBuilder.WriteString(fmt.Sprintf("📊 页数：%d/%d\n\n", page+1, totalPages))
		
		for _, order := range orders {
			status := b.msg.Get(lang, "order_status_"+order.Status)
			
			// Handle product name safely
			productName := "充值"
			if order.Product != nil {
				productName = order.Product.Name
			}
			
			// Get code for this order
			code, err := store.GetOrderCode(b.db, order.ID)
			if err != nil {
				logger.Error("Failed to get order code", "error", err, "order_id", order.ID)
				code = "N/A"
			}
			
			orderInfo := fmt.Sprintf(
				"🆔 #%d | %s\n📦 %s\n💰 %s%.2f\n🔑 卡密：`%s`\n🕐 %s\n\n",
				order.ID,
				status,
				productName,
				currencySymbol,
				float64(order.AmountCents)/100,
				code,
				order.CreatedAt.Format("01/02 15:04"),
			)
			msgBuilder.WriteString(orderInfo)
		}
	}
	
	// Create pagination keyboard
	var keyboardRows [][]tgbotapi.InlineKeyboardButton
	
	// Add pagination buttons if there are multiple pages
	totalPages := int((totalCount + ordersPerPage - 1) / ordersPerPage)
	if totalPages > 1 {
		var paginationRow []tgbotapi.InlineKeyboardButton
		
		// Previous button
		if page > 0 {
			paginationRow = append(paginationRow, 
				tgbotapi.NewInlineKeyboardButtonData("⬅️ 上一页", fmt.Sprintf("orders_page:%d", page-1)))
		}
		
		// Page number
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		
		// Next button
		if page < totalPages-1 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("下一页 ➡️", fmt.Sprintf("orders_page:%d", page+1)))
		}
		
		keyboardRows = append(keyboardRows, paginationRow)
	}
	
	// Create keyboard
	keyboard := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, msgBuilder.String())
	if len(keyboardRows) > 0 {
		msg.ReplyMarkup = keyboard
	}
	msg.ParseMode = "Markdown"
	
	if _, err := b.api.Send(msg); err != nil {
		logger.Error("Failed to send my orders message", "error", err, "user_id", user.ID)
	} else {
		logger.Info("My orders message sent successfully", "user_id", user.ID, "orders_count", len(orders), "page", page)
	}
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
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
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
	// Handle product name safely
	productName := "充值"
	if order.Product != nil {
		productName = order.Product.Name
	}
	
	msgBuilder.WriteString(b.msg.Format(lang, "order_details", map[string]interface{}{
		"Currency":    currencySymbol,
		"ProductName": productName,
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

// handleMyOrdersPageEdit handles pagination callbacks for orders
func (b *Bot) handleMyOrdersPageEdit(callback *tgbotapi.CallbackQuery, page int) {
	logger.Info("handleMyOrdersPageEdit called", "from", callback.From.ID, "page", page)
	
	// Get user
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		b.api.Request(tgbotapi.NewCallback(callback.ID, "Error"))
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	// Constants for pagination
	const ordersPerPage = 5
	offset := page * ordersPerPage
	
	// Get total count of paid orders
	totalCount, err := store.GetUserPaidOrderCount(b.db, user.ID)
	if err != nil {
		logger.Error("Failed to get paid order count", "error", err)
		b.api.Request(tgbotapi.NewCallback(callback.ID, b.msg.Get(lang, "failed_to_load_orders")))
		return
	}
	
	// Get user's paid orders for current page
	orders, err := store.GetUserPaidOrders(b.db, user.ID, ordersPerPage, offset)
	if err != nil {
		logger.Error("Failed to get user paid orders", "error", err)
		b.api.Request(tgbotapi.NewCallback(callback.ID, b.msg.Get(lang, "failed_to_load_orders")))
		return
	}
	
	// Build order list message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(b.msg.Get(lang, "my_orders_title"))
	msgBuilder.WriteString("\n\n")
	
	if totalCount == 0 {
		msgBuilder.WriteString(b.msg.Get(lang, "no_orders_yet"))
	} else {
		// Show page info
		totalPages := int((totalCount + ordersPerPage - 1) / ordersPerPage)
		msgBuilder.WriteString(fmt.Sprintf("📊 页数：%d/%d\n\n", page+1, totalPages))
		
		for _, order := range orders {
			status := b.msg.Get(lang, "order_status_"+order.Status)
			
			// Handle product name safely
			productName := "充值"
			if order.Product != nil {
				productName = order.Product.Name
			}
			
			// Get code for this order
			code, err := store.GetOrderCode(b.db, order.ID)
			if err != nil {
				logger.Error("Failed to get order code", "error", err, "order_id", order.ID)
				code = "N/A"
			}
			
			orderInfo := fmt.Sprintf(
				"🆔 #%d | %s\n📦 %s\n💰 %s%.2f\n🔑 卡密：`%s`\n🕐 %s\n\n",
				order.ID,
				status,
				productName,
				currencySymbol,
				float64(order.AmountCents)/100,
				code,
				order.CreatedAt.Format("01/02 15:04"),
			)
			msgBuilder.WriteString(orderInfo)
		}
	}
	
	// Create pagination keyboard
	var keyboardRows [][]tgbotapi.InlineKeyboardButton
	
	// Add pagination buttons if there are multiple pages
	totalPages := int((totalCount + ordersPerPage - 1) / ordersPerPage)
	if totalPages > 1 {
		var paginationRow []tgbotapi.InlineKeyboardButton
		
		// Previous button
		if page > 0 {
			paginationRow = append(paginationRow, 
				tgbotapi.NewInlineKeyboardButtonData("⬅️ 上一页", fmt.Sprintf("orders_page:%d", page-1)))
		}
		
		// Page number
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		
		// Next button
		if page < totalPages-1 {
			paginationRow = append(paginationRow,
				tgbotapi.NewInlineKeyboardButtonData("下一页 ➡️", fmt.Sprintf("orders_page:%d", page+1)))
		}
		
		keyboardRows = append(keyboardRows, paginationRow)
	}
	
	// Create keyboard
	keyboard := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	
	// Edit the message
	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		msgBuilder.String(),
	)
	editMsg.ParseMode = "Markdown"
	if len(keyboardRows) > 0 {
		editMsg.ReplyMarkup = &keyboard
	}
	
	if _, err := b.api.Send(editMsg); err != nil {
		logger.Error("Failed to edit orders message", "error", err)
	}
	
	// Answer the callback
	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
}