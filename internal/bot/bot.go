package bot

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
	"shop-bot/internal/payment/epay"
	"shop-bot/internal/config"
	"shop-bot/internal/bot/messages"
	"shop-bot/internal/metrics"
	"shop-bot/internal/broadcast"
	"shop-bot/internal/notification"
	"gorm.io/gorm"
)

type Bot struct {
	api       *tgbotapi.BotAPI
	db        *gorm.DB
	epay      *epay.Client
	config    *config.Config
	msg       *messages.Manager
	broadcast *broadcast.Service
	notification *notification.Service
	ticketService TicketService // Remove pointer - interface should not be pointer
	
	// User state management
	userStates     map[int64]string
	userStatesMutex sync.RWMutex
}

// TicketService interface to avoid circular imports
type TicketService interface {
	GetTicketByUserMessage(userID int64) (*store.Ticket, error)
	AddMessage(ticketID uint, senderType string, senderID int64, senderName, content string, messageID int) error
	CreateTicket(userID int64, username, subject, category, content string) (*store.Ticket, error)
}

func New(token string, db *gorm.DB) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot api: %w", err)
	}
	
	// Load config for epay and base URL
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// Initialize epay client if configured
	var epayClient *epay.Client
	if cfg.EpayPID != "" && cfg.EpayKey != "" && cfg.EpayGateway != "" {
		epayClient = epay.NewClient(cfg.EpayPID, cfg.EpayKey, cfg.EpayGateway)
	}
	
	// Initialize notification service
	notificationService := notification.NewService(api, cfg, db)

	return &Bot{
		api:    api,
		db:     db,
		epay:   epayClient,
		config: cfg,
		msg:    messages.GetManager(),
		broadcast: broadcast.NewService(db, api),
		notification: notificationService,
		userStates: make(map[int64]string),
	}, nil
}

// SetTicketService sets the ticket service for the bot
func (b *Bot) SetTicketService(service TicketService) {
	b.ticketService = service
}

// GetAPI returns the telegram bot API instance
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) Start(ctx context.Context) error {
	if b.config.UseWebhook {
		// In webhook mode, updates will be handled by HTTP server
		logger.Info("Bot configured for webhook mode")
		return nil
	}
	return b.startPolling(ctx)
}

func (b *Bot) startPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	logger.Info("Bot started in polling mode", "username", b.api.Self.UserName)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			go b.handleUpdate(update)
		}
	}
}


// HandleWebhookUpdate handles webhook updates
func (b *Bot) HandleWebhookUpdate(update tgbotapi.Update) {
	b.handleUpdate(update)
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	// Log update details
	logger.Info("Processing update", "update_id", update.UpdateID,
		"has_message", update.Message != nil,
		"has_callback", update.CallbackQuery != nil)
	
	// Handle callback queries (inline keyboard buttons)
	if update.CallbackQuery != nil {
		metrics.BotMessagesReceived.WithLabelValues("callback").Inc()
		b.handleCallbackQuery(update.CallbackQuery)
		return
	}
	
	// Handle regular messages
	if update.Message == nil {
		return
	}

	// Check if it's a group message
	if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
		metrics.BotMessagesReceived.WithLabelValues("group").Inc()
		b.handleGroupMessage(update.Message)
		return
	}

	// Handle commands
	if update.Message.IsCommand() {
		metrics.BotMessagesReceived.WithLabelValues("command").Inc()
		switch update.Message.Command() {
		case "start":
			b.handleStart(update.Message)
		}
		return
	}
	
	// Handle text messages (ReplyKeyboard buttons)
	if update.Message.Text != "" {
		metrics.BotMessagesReceived.WithLabelValues("text").Inc()
		logger.Info("Handling text message", "text", update.Message.Text, "from", update.Message.From.ID)
		b.handleTextMessage(update.Message)
	}
}

func (b *Bot) handleStart(message *tgbotapi.Message) {
	// Get or create user
	langCode := message.From.LanguageCode
	user, isNew, err := store.GetOrCreateUserWithStatus(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get/create user", "error", err, "tg_user_id", message.From.ID)
		return
	}
	
	// If it's a new user, send notification to admins
	if isNew && b.notification != nil {
		b.notification.NotifyAdmins(notification.EventNewUser, map[string]interface{}{
			"user_id":   user.ID,
			"tg_user_id": user.TgUserID,
			"username":  user.Username,
		})
	}
	
	// Determine user language
	lang := messages.GetUserLanguage(user.Language, langCode)
	
	// Update user language if needed
	if user.Language == "" && langCode != "" {
		detectedLang := "en"
		if strings.HasPrefix(langCode, "zh") {
			detectedLang = "zh"
		}
		b.db.Model(&user).Update("language", detectedLang)
		user.Language = detectedLang
		lang = detectedLang
	}
	
	// Create reply keyboard with localized buttons
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_buy")),
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_deposit")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_profile")),
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_orders")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_faq")),
			tgbotapi.NewKeyboardButton(b.msg.Get(lang, "btn_support")),
		),
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "start_title"))
	msg.ReplyMarkup = keyboard
	
	if _, err := b.api.Send(msg); err != nil {
		logger.Error("Failed to send message", "error", err, "chat_id", message.Chat.ID)
	}
	
	logger.Info("User started bot", "user_id", user.ID, "tg_user_id", user.TgUserID)
}

func (b *Bot) handleTextMessage(message *tgbotapi.Message) {
	// Get user for language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)

	// Log the received message text for debugging
	logger.Info("Received text message",
		"text", message.Text,
		"user_id", user.ID,
		"telegram_id", message.From.ID,
		"is_reply", message.ReplyToMessage != nil)

	// First check if this is an admin
	var admin store.AdminUser
	isAdmin := false
	telegramID := message.From.ID
	err := b.db.Where("telegram_id = ? AND is_active = true", telegramID).First(&admin).Error
	if err == nil {
		isAdmin = true
		logger.Info("Message from admin", "admin_id", admin.ID, "admin_username", admin.Username, "telegram_id", telegramID)
	} else {
		logger.Info("Not an admin message", "telegram_id", telegramID, "error", err)
	}

	// Check if this is an admin replying to a ticket notification
	if isAdmin && b.isAdminReplyToTicket(message) {
		b.handleAdminTicketReply(message)
		return
	}

	// Check if user is in custom deposit state
	b.userStatesMutex.RLock()
	userState, hasState := b.userStates[message.From.ID]
	b.userStatesMutex.RUnlock()
	
	if hasState && userState == "awaiting_deposit_amount" {
		// Handle custom deposit amount
		b.handleCustomDepositAmount(message)
		return
	}
	
	// Check against localized button texts
	switch message.Text {
	case b.msg.Get(lang, "btn_buy"), "Buy", "购买":
		b.handleBuy(message)
		return
	case b.msg.Get(lang, "btn_deposit"), "Deposit", "充值":
		b.handleDeposit(message)
		return
	case b.msg.Get(lang, "btn_profile"), "Profile", "个人信息":
		b.handleProfile(message)
		return
	case b.msg.Get(lang, "btn_orders"), "Orders", "My Orders", "我的订单":
		logger.Info("Handling my orders request", "user_id", user.ID)
		b.handleMyOrders(message)
		return
	case b.msg.Get(lang, "btn_faq"), "FAQ", "常见问题":
		b.handleFAQ(message)
		return
	case b.msg.Get(lang, "btn_support"), "Support", "客服支持":
		b.handleSupportButton(message)
		return
	case "/language":
		b.handleLanguageSelection(message)
		return
	case "/ticket", "/support", "客服":
		b.handleSupportCommand(message)
		return
	}

	// Check if it's a recharge card code (starts with specific prefix)
	if strings.HasPrefix(message.Text, "RC-") || strings.HasPrefix(message.Text, "充值卡-") {
		b.handleRechargeCard(message)
		return
	}

	// For admins, don't process as ticket message
	if isAdmin {
		logger.Info("Admin message not handled by any specific handler", "text", message.Text)
		// Don't send help message to admins
		return
	}

	// Check if user has an active ticket - treat any other message as ticket message
	if b.ticketService != nil {
		logger.Info("Checking for active ticket", "user_id", message.From.ID)
		if ticket, err := b.ticketService.GetTicketByUserMessage(message.From.ID); err == nil && ticket != nil {
			// User has an active ticket, add message to it
			logger.Info("Found active ticket", "ticket_id", ticket.ID, "ticket_number", ticket.TicketID)
			username := message.From.UserName
			if username == "" {
				username = fmt.Sprintf("User %d", message.From.ID)
			}

			err := b.ticketService.AddMessage(ticket.ID, "user", message.From.ID, username, message.Text, message.MessageID)
			if err != nil {
				logger.Error("Failed to add message to ticket", "error", err, "ticket_id", ticket.ID)
			} else {
				logger.Info("Message added to ticket successfully", "ticket_id", ticket.ID, "message", message.Text)
				// Send confirmation to user
				confirmMsg := b.msg.Get(lang, "support_message_sent")
				reply := tgbotapi.NewMessage(message.Chat.ID, confirmMsg)
				b.api.Send(reply)
			}
			return
		} else {
			logger.Info("No active ticket found", "user_id", message.From.ID, "error", err)
		}
	} else {
		logger.Warn("Ticket service is nil")
	}

	// No active ticket - show help message
	logger.Info("Unhandled message text", "text", message.Text, "user_id", user.ID)
	helpMsg := b.msg.Get(lang, "help_message")
	if helpMsg == "help_message" { // Fallback if template not found
		helpMsg = "请选择一个选项或发送 /ticket 联系客服。\nPlease select an option or send /ticket to contact support."
	}
	reply := tgbotapi.NewMessage(message.Chat.ID, helpMsg)
	b.api.Send(reply)
}

func (b *Bot) handleBuy(message *tgbotapi.Message) {
	// Get user for language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Get active products
	products, err := store.GetActiveProducts(b.db)
	if err != nil {
		logger.Error("Failed to get products", "error", err)
		b.sendError(message.Chat.ID, b.msg.Format(lang, "failed_to_load", map[string]string{"Item": "products"}))
		return
	}
	
	if len(products) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "no_products"))
		b.api.Send(msg)
		return
	}
	
	// Create inline keyboard with products
	var rows [][]tgbotapi.InlineKeyboardButton
	
	for _, product := range products {
		// Get available stock
		stock, err := store.CountAvailableCodes(b.db, product.ID)
		if err != nil {
			logger.Error("Failed to count stock", "error", err, "product_id", product.ID)
			stock = 0
		}
		
		// Format button text: "Name - $Price (Stock)"
		// Get currency symbol
		_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
		
		buttonText := fmt.Sprintf("%s - %s%.2f (%d)", 
			product.Name, 
			currencySymbol,
			float64(product.PriceCents)/100, 
			stock,
		)
		
		callbackData := fmt.Sprintf("buy:%d", product.ID)
		
		button := tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "buy_tips"))
	msg.ReplyMarkup = keyboard
	
	if _, err := b.api.Send(msg); err != nil {
		logger.Error("Failed to send product list", "error", err)
	}
}

func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery) {
	// Acknowledge the callback
	callbackConfig := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(callbackConfig); err != nil {
		logger.Error("Failed to answer callback", "error", err)
	}
	
	// Parse callback data
	if strings.HasPrefix(callback.Data, "buy:") {
		productIDStr := strings.TrimPrefix(callback.Data, "buy:")
		productID, err := strconv.ParseUint(productIDStr, 10, 32)
		if err != nil {
			logger.Error("Invalid product ID", "error", err, "data", callback.Data)
			return
		}
		
		b.handleBuyProduct(callback, uint(productID))
	} else if strings.HasPrefix(callback.Data, "confirm_buy:") {
		// Format: confirm_buy:productID:useBalance(1/0)
		parts := strings.Split(callback.Data, ":")
		if len(parts) == 3 {
			productID, _ := strconv.ParseUint(parts[1], 10, 32)
			useBalance := parts[2] == "1"
			b.handleConfirmBuy(callback, uint(productID), useBalance)
		}
	} else if callback.Data == "select_language" {
		b.handleLanguageSelection(callback.Message)
	} else if strings.HasPrefix(callback.Data, "set_lang:") {
		lang := strings.TrimPrefix(callback.Data, "set_lang:")
		b.handleSetLanguage(callback, lang)
	} else if strings.HasPrefix(callback.Data, "lang:") {
		// Handle language switch from FAQ
		parts := strings.Split(callback.Data, ":")
		if len(parts) >= 2 {
			newLang := parts[1]
			// Update user language
			user, _ := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
			b.db.Model(&user).Update("language", newLang)

			// Send a message with updated menu in new language
			// Create reply keyboard with localized buttons
			keyboard := tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_buy")),
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_deposit")),
				),
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_profile")),
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_orders")),
				),
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_faq")),
					tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_support")),
				),
			)

			// Send language change confirmation with new menu
			langChangeMsg := b.msg.Get(newLang, "language_changed")
			if langChangeMsg == "language_changed" {
				// Fallback message if template not found
				if newLang == "zh" {
					langChangeMsg = "✅ 语言已切换为中文"
				} else {
					langChangeMsg = "✅ Language changed to English"
				}
			}

			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, langChangeMsg)
			msg.ReplyMarkup = keyboard
			b.api.Send(msg)

			// If it's from FAQ, also show FAQ in new language
			if len(parts) == 3 && parts[2] == "faq" {
				// Get FAQs in new language
				faqs, err := store.GetActiveFAQs(b.db, newLang)
				faqTitle := b.msg.Get(newLang, "faq_title")
				var faqContent string

				if err != nil || len(faqs) == 0 {
					faqContent = b.msg.Get(newLang, "faq_content")
				} else {
					for i, faq := range faqs {
						if i > 0 {
							faqContent += "\n\n"
						}
						faqContent += fmt.Sprintf("❓ *%s*\n%s", escapeMarkdown(faq.Question), escapeMarkdown(faq.Answer))
					}
				}

				// Create new keyboard with opposite language
				switchToLang := "zh"
				switchToLabel := "🇨🇳 中文"
				if newLang == "zh" {
					switchToLang = "en"
					switchToLabel = "🇬🇧 English"
				}

				inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData(switchToLabel, fmt.Sprintf("lang:%s:faq", switchToLang)),
					),
				)

				// Send FAQ message
				faqMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, faqTitle+"\n\n"+faqContent)
				faqMsg.ParseMode = "Markdown"
				faqMsg.ReplyMarkup = inlineKeyboard
				b.api.Send(faqMsg)
			}
		}
	} else if callback.Data == "balance_history" {
		b.handleBalanceHistory(callback)
	} else if strings.HasPrefix(callback.Data, "group_toggle_") {
		b.handleGroupToggle(callback)
	} else if callback.Data == "my_orders" || callback.Data == "order_list" {
		// Convert callback to message for reuse
		msg := &tgbotapi.Message{
			Chat: callback.Message.Chat,
			From: callback.From,
		}
		b.handleMyOrders(msg)
	} else if strings.HasPrefix(callback.Data, "orders_page:") {
		// Handle pagination for orders
		pageStr := strings.TrimPrefix(callback.Data, "orders_page:")
		page, err := strconv.Atoi(pageStr)
		if err == nil {
			// Edit the existing message with new page
			b.handleMyOrdersPageEdit(callback, page)
		}
	} else if callback.Data == "noop" {
		// No operation - just acknowledge the callback
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		return
	} else if strings.HasPrefix(callback.Data, "order:") {
		orderIDStr := strings.TrimPrefix(callback.Data, "order:")
		var orderID uint
		fmt.Sscanf(orderIDStr, "%d", &orderID)
		if orderID > 0 {
			b.handleOrderDetails(callback, orderID)
		}
	} else if strings.HasPrefix(callback.Data, "deposit_") {
		b.handleDepositCallback(callback)
	}
}

func (b *Bot) handleBuyProduct(callback *tgbotapi.CallbackQuery, productID uint) {
	// Get user
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		lang := messages.GetUserLanguage("", callback.From.LanguageCode)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_process"))
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	// Get product
	product, err := store.GetProduct(b.db, productID)
	if err != nil {
		logger.Error("Failed to get product", "error", err, "product_id", productID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "product_not_found"))
		return
	}
	
	// Check stock
	stock, err := store.CountAvailableCodes(b.db, productID)
	if err != nil || stock == 0 {
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, b.msg.Get(lang, "out_of_stock"))
		b.api.Send(msg)
		
		// Update the inline keyboard to reflect new stock
		go b.UpdateInlineStock(callback.Message.Chat.ID, callback.Message.MessageID)
		return
	}
	
	// Get user balance
	balance, _ := store.GetUserBalance(b.db, user.ID)
	
	// Check if user has balance and offer to use it
	if balance > 0 {
		// Calculate how much balance can be used
		balanceUsed := 0
		paymentAmount := product.PriceCents
		
		if balance >= product.PriceCents {
			balanceUsed = product.PriceCents
			paymentAmount = 0
		} else {
			balanceUsed = balance
			paymentAmount = product.PriceCents - balance
		}
		
		// Ask user if they want to use balance
		balanceMsg := b.msg.Format(lang, "use_balance_prompt", map[string]interface{}{
			"Currency": currencySymbol,
			"Balance": fmt.Sprintf("%.2f", float64(balance)/100),
			"Product": product.Name,
			"Price": fmt.Sprintf("%.2f", float64(product.PriceCents)/100),
			"BalanceUsed": fmt.Sprintf("%.2f", float64(balanceUsed)/100),
			"ToPay": fmt.Sprintf("%.2f", float64(paymentAmount)/100),
		})
		
		// Create inline keyboard for balance usage choice
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(b.msg.Get(lang, "use_balance_yes"), fmt.Sprintf("confirm_buy:%d:1", productID)),
				tgbotapi.NewInlineKeyboardButtonData(b.msg.Get(lang, "use_balance_no"), fmt.Sprintf("confirm_buy:%d:0", productID)),
			),
		)
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, balanceMsg)
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
		return
	}
	
	// No balance, proceed directly to create order
	b.handleConfirmBuy(callback, productID, false)
}

func (b *Bot) handleConfirmBuy(callback *tgbotapi.CallbackQuery, productID uint, useBalance bool) {
	// Get user
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		lang := messages.GetUserLanguage("", callback.From.LanguageCode)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_process"))
		return
	}

	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)

	// Get product
	product, err := store.GetProduct(b.db, productID)
	if err != nil {
		logger.Error("Failed to get product", "error", err, "product_id", productID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "product_not_found"))
		return
	}

	// Create order with or without balance
	var order *store.Order
	if useBalance {
		order, err = store.CreateOrderWithBalance(b.db, user.ID, product.ID, product.PriceCents, true)
	} else {
		order, err = store.CreateOrder(b.db, user.ID, product.ID, product.PriceCents)
	}
	
	if err != nil {
		logger.Error("Failed to create order", "error", err)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_order"))
		return
	}

	// Track order created metric
	metrics.OrdersCreated.Inc()
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)

	// If payment amount is 0 (fully paid with balance), deliver immediately
	if order.PaymentAmount == 0 {
		// Try to claim and deliver code
		ctx := context.Background()
		code, err := store.ClaimOneCodeTx(ctx, b.db, product.ID, order.ID)
		if err != nil {
			logger.Error("Failed to claim code", "error", err, "order_id", order.ID)
			
			// Update order status to failed_delivery
			b.db.Model(order).Update("status", "failed_delivery")
			
			// Send no stock message
			noStockMsg := b.msg.Format(lang, "no_stock", map[string]interface{}{
				"OrderID":     order.ID,
				"ProductName": product.Name,
			})
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, noStockMsg)
			b.api.Send(msg)
			return
		}

		// Update order status to delivered
		now := time.Now()
		b.db.Model(order).Updates(map[string]interface{}{
			"status": "delivered",
			"delivered_at": &now,
		})

		// Send code to user
		deliveryMsg := b.msg.Format(lang, "order_paid", map[string]interface{}{
			"OrderID":     order.ID,
			"ProductName": product.Name,
			"Code":        code,
		})
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, deliveryMsg)
		msg.ParseMode = "Markdown"
		b.api.Send(msg)
		
		logger.Info("Order paid with balance and delivered", "order_id", order.ID, "user_id", user.ID, "product_id", product.ID)
		return
	}

	// Generate out_trade_no for payment with nanosecond precision to avoid duplicates
	outTradeNo := fmt.Sprintf("%d-%d", order.ID, time.Now().UnixNano())

	// Update order with out_trade_no
	if err := b.db.Model(&store.Order{}).Where("id = ?", order.ID).Update("epay_out_trade_no", outTradeNo).Error; err != nil {
		logger.Error("Failed to update order out_trade_no", "error", err, "order_id", order.ID)
	}

	// Check if payment is configured
	if b.epay == nil {
		orderMsg := b.msg.Format(lang, "order_created", map[string]interface{}{
			"Currency":    currencySymbol,
			"ProductName": product.Name,
			"Price":       fmt.Sprintf("%.2f", float64(order.PaymentAmount)/100),
			"OrderID":     order.ID,
		})
		
		if order.BalanceUsed > 0 {
			orderMsg += "\n" + b.msg.Format(lang, "balance_used_info", map[string]interface{}{
				"Currency":    currencySymbol,
				"BalanceUsed": fmt.Sprintf("%.2f", float64(order.BalanceUsed)/100),
			})
		}
		
		orderMsg += "\n\n" + b.msg.Get(lang, "payment_not_configured")
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		b.api.Send(msg)
		return
	}

	// Create payment order using submit URL (allows user to choose payment method)
	notifyURL := fmt.Sprintf("%s/payment/epay/notify", b.config.BaseURL)
	returnURL := fmt.Sprintf("%s/payment/return", b.config.BaseURL)

	// Create submit URL for payment page
	payURL := b.epay.CreateSubmitURL(epay.CreateOrderParams{
		OutTradeNo: outTradeNo,
		Name:       product.Name,
		Money:      float64(order.PaymentAmount) / 100, // Use payment amount after balance deduction
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		Param:      fmt.Sprintf("user_%d", user.ID), // Store user ID for reference
	})

	// Send payment message with inline button
	orderMsg := b.msg.Format(lang, "order_created", map[string]interface{}{
		"ProductName": product.Name,
		"Price":       fmt.Sprintf("%.2f", float64(order.PaymentAmount)/100),
		"OrderID":     order.ID,
	})
	
	if order.BalanceUsed > 0 {
		orderMsg += "\n" + b.msg.Format(lang, "balance_used_info", map[string]interface{}{
			"BalanceUsed": fmt.Sprintf("%.2f", float64(order.BalanceUsed)/100),
		})
	}

	// Send payment message with inline button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(b.msg.Get(lang, "pay_now"), payURL),
		),
	)
	
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)

	logger.Info("Order created", "order_id", order.ID, "user_id", user.ID, "product_id", product.ID, "balance_used", order.BalanceUsed)
}

func (b *Bot) handleDeposit(message *tgbotapi.Message) {
	// Get user for language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Get current balance
	balance, _ := store.GetUserBalance(b.db, user.ID)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	depositMsg := b.msg.Format(lang, "deposit_info", map[string]interface{}{
		"Currency": currencySymbol,
		"Balance": fmt.Sprintf("%.2f", float64(balance)/100),
	})
	
	// Add deposit options
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("💵 %s10", currencySymbol), "deposit_10"),
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("💵 %s20", currencySymbol), "deposit_20"),
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("💵 %s50", currencySymbol), "deposit_50"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("💵 %s100", currencySymbol), "deposit_100"),
			tgbotapi.NewInlineKeyboardButtonData("🔢 "+b.msg.Get(lang, "custom_amount"), "deposit_custom"),
		),
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, depositMsg)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

func (b *Bot) handleDepositCallback(callback *tgbotapi.CallbackQuery) {
	// Get user for language
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	// Check if payment is configured
	if b.epay == nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, b.msg.Get(lang, "payment_not_configured")))
		return
	}
	
	// Parse deposit amount
	var amountCents int
	switch callback.Data {
	case "deposit_10":
		amountCents = 1000
	case "deposit_20":
		amountCents = 2000
	case "deposit_50":
		amountCents = 5000
	case "deposit_100":
		amountCents = 10000
	case "deposit_custom":
		// Set user state to awaiting deposit amount
		b.userStatesMutex.Lock()
		b.userStates[callback.From.ID] = "awaiting_deposit_amount"
		b.userStatesMutex.Unlock()
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, b.msg.Get(lang, "custom_amount_instruction"))
		b.api.Send(msg)
		b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
		return
	default:
		return
	}
	
	// Create a deposit order
	order, err := store.CreateDepositOrder(b.db, user.ID, amountCents)
	if err != nil {
		logger.Error("Failed to create deposit order", "error", err)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_order"))
		return
	}
	
	// Generate payment URL with nanosecond precision to avoid duplicates
	outTradeNo := fmt.Sprintf("D%d-%d", order.ID, time.Now().UnixNano())
	
	// Update order with out_trade_no
	if err := b.db.Model(&store.Order{}).Where("id = ?", order.ID).Update("epay_out_trade_no", outTradeNo).Error; err != nil {
		logger.Error("Failed to update order out_trade_no", "error", err, "order_id", order.ID)
	}
	
	// Create payment order using submit URL (allows user to choose payment method)
	notifyURL := fmt.Sprintf("%s/payment/epay/notify", b.config.BaseURL)
	returnURL := fmt.Sprintf("%s/payment/return", b.config.BaseURL)
	
	// Create submit URL for payment page
	payURL := b.epay.CreateSubmitURL(epay.CreateOrderParams{
		OutTradeNo: outTradeNo,
		Name:       fmt.Sprintf("充值 %s%.2f", currencySymbol, float64(amountCents)/100),
		Money:      float64(amountCents) / 100,
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		Param:      fmt.Sprintf("deposit_%d", user.ID),
	})
	
	// Send payment message
	depositMsg := b.msg.Format(lang, "deposit_order_created", map[string]interface{}{
		"Currency": currencySymbol,
		"Amount":  fmt.Sprintf("%.2f", float64(amountCents)/100),
		"OrderID": order.ID,
	})
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(b.msg.Get(lang, "pay_now"), payURL),
		),
	)
	
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, depositMsg)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
	
	logger.Info("Deposit order created", "order_id", order.ID, "user_id", user.ID, "amount", amountCents)
}

func (b *Bot) handleProfile(message *tgbotapi.Message) {
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		lang := messages.GetUserLanguage("", message.From.LanguageCode)
		b.sendError(message.Chat.ID, b.msg.Format(lang, "failed_to_load", map[string]string{"Item": "profile"}))
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)

	// Get user balance
	balance, _ := store.GetUserBalance(b.db, user.ID)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	profileMsg := b.msg.Format(lang, "profile_info", map[string]interface{}{
		"UserID":     user.TgUserID,
		"Username":   user.Username,
		"Language":   user.Language,
		"JoinedDate": user.CreatedAt.Format("2006-01-02"),
		"Currency":   currencySymbol,
		"Balance":    fmt.Sprintf("%.2f", float64(balance)/100),
	})
	
	// Add language selection button
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Change Language / 切换语言", "select_language"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.msg.Get(lang, "view_balance_history"), "balance_history"),
		),
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "profile_title")+"\n\n"+profileMsg)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleFAQ(message *tgbotapi.Message) {
	// Get user for language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)

	// Get FAQs from database
	faqs, err := store.GetActiveFAQs(b.db, lang)
	if err != nil {
		logger.Error("Failed to get FAQs", "error", err, "language", lang)
		// Fall back to static content
		faqContent := b.msg.Get(lang, "faq_content")
		faqTitle := b.msg.Get(lang, "faq_title")
		msg := tgbotapi.NewMessage(message.Chat.ID, faqTitle+"\n\n"+faqContent)
		b.api.Send(msg)
		return
	}

	// Build FAQ message
	faqTitle := b.msg.Get(lang, "faq_title")
	var faqContent string

	if len(faqs) == 0 {
		// No FAQs found, use default message
		faqContent = b.msg.Get(lang, "faq_content")
	} else {
		// Format FAQs
		for i, faq := range faqs {
			if i > 0 {
				faqContent += "\n\n"
			}
			faqContent += fmt.Sprintf("❓ *%s*\n%s", escapeMarkdown(faq.Question), escapeMarkdown(faq.Answer))
		}
	}

	// Create inline keyboard with language switch button
	var keyboard tgbotapi.InlineKeyboardMarkup

	// Determine the opposite language
	switchToLang := "zh"
	switchToLabel := "🇨🇳 中文"
	if lang == "zh" {
		switchToLang = "en"
		switchToLabel = "🇬🇧 English"
	}

	keyboard = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(switchToLabel, fmt.Sprintf("lang:%s:faq", switchToLang)),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, faqTitle+"\n\n"+faqContent)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) sendError(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, "❌ "+text)
	b.api.Send(msg)
}

// UpdateInlineStock updates the stock numbers in an inline keyboard message
func (b *Bot) UpdateInlineStock(chatID int64, messageID int) error {
	// Get active products
	products, err := store.GetActiveProducts(b.db)
	if err != nil {
		return err
	}
	
	// Recreate inline keyboard with updated stock
	var rows [][]tgbotapi.InlineKeyboardButton
	
	for _, product := range products {
		stock, err := store.CountAvailableCodes(b.db, product.ID)
		if err != nil {
			stock = 0
		}
		
		// Get currency symbol
		_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
		
		buttonText := fmt.Sprintf("%s - %s%.2f (%d)", 
			product.Name, 
			currencySymbol,
			float64(product.PriceCents)/100, 
			stock,
		)
		
		callbackData := fmt.Sprintf("buy:%d", product.ID)
		button := tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	
	editMsg := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, keyboard)
	_, err = b.api.Send(editMsg)
	
	return err
}


// GetBroadcastService returns the broadcast service
func (b *Bot) GetBroadcastService() *broadcast.Service {
	return b.broadcast
}

// SetWebhook sets the webhook URL
func (b *Bot) SetWebhook(webhookURL string) error {
	webhook, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}
	
	_, err = b.api.Request(webhook)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}
	
	logger.Info("Webhook set successfully", "url", webhookURL)
	return nil
}

// RemoveWebhook removes the webhook
func (b *Bot) RemoveWebhook() error {
	deleteWebhook := tgbotapi.DeleteWebhookConfig{
		DropPendingUpdates: false,
	}
	
	_, err := b.api.Request(deleteWebhook)
	if err != nil {
		return fmt.Errorf("failed to remove webhook: %w", err)
	}
	
	logger.Info("Webhook removed successfully")
	return nil
}

func (b *Bot) handleCustomDepositAmount(message *tgbotapi.Message) {
	// Clear user state
	b.userStatesMutex.Lock()
	delete(b.userStates, message.From.ID)
	b.userStatesMutex.Unlock()
	
	// Get user for language
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Check if payment is configured
	if b.epay == nil {
		b.sendError(message.Chat.ID, b.msg.Get(lang, "payment_not_configured"))
		return
	}
	
	// Parse amount from message
	amountStr := strings.TrimSpace(message.Text)
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 请输入有效的金额，例如：30")
		b.api.Send(msg)
		
		// Set state again to allow retry
		b.userStatesMutex.Lock()
		b.userStates[message.From.ID] = "awaiting_deposit_amount"
		b.userStatesMutex.Unlock()
		return
	}
	
	// Convert to cents
	amountCents := int(amount * 100)
	
	// Check minimum and maximum limits
	if amountCents < 100 { // Minimum $1
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 最低充值金额为 1 元")
		b.api.Send(msg)
		
		// Set state again to allow retry
		b.userStatesMutex.Lock()
		b.userStates[message.From.ID] = "awaiting_deposit_amount"
		b.userStatesMutex.Unlock()
		return
	}
	
	if amountCents > 1000000 { // Maximum $10,000
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 最高充值金额为 10,000 元")
		b.api.Send(msg)
		
		// Set state again to allow retry
		b.userStatesMutex.Lock()
		b.userStates[message.From.ID] = "awaiting_deposit_amount"
		b.userStatesMutex.Unlock()
		return
	}
	
	// Create a deposit order
	order, err := store.CreateDepositOrder(b.db, user.ID, amountCents)
	if err != nil {
		logger.Error("Failed to create deposit order", "error", err)
		b.sendError(message.Chat.ID, b.msg.Get(lang, "failed_to_create_order"))
		return
	}
	
	// Generate payment URL with nanosecond precision to avoid duplicates
	outTradeNo := fmt.Sprintf("D%d-%d", order.ID, time.Now().UnixNano())
	
	// Update order with out_trade_no
	if err := b.db.Model(&store.Order{}).Where("id = ?", order.ID).Update("epay_out_trade_no", outTradeNo).Error; err != nil {
		logger.Error("Failed to update order out_trade_no", "error", err, "order_id", order.ID)
	}
	
	// Create payment order using submit URL (allows user to choose payment method)
	notifyURL := fmt.Sprintf("%s/payment/epay/notify", b.config.BaseURL)
	returnURL := fmt.Sprintf("%s/payment/return", b.config.BaseURL)
	
	// Get currency symbol
	_, currencySymbol := store.GetCurrencySettings(b.db, b.config)
	
	// Create submit URL for payment page
	payURL := b.epay.CreateSubmitURL(epay.CreateOrderParams{
		OutTradeNo: outTradeNo,
		Name:       fmt.Sprintf("充值 %s%.2f", currencySymbol, float64(amountCents)/100),
		Money:      float64(amountCents) / 100,
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		Param:      fmt.Sprintf("deposit_%d", user.ID),
	})
	
	// Send payment message
	depositMsg := b.msg.Format(lang, "deposit_order_created", map[string]interface{}{
		"Currency": currencySymbol,
		"Amount":  fmt.Sprintf("%.2f", float64(amountCents)/100),
		"OrderID": order.ID,
	})
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(b.msg.Get(lang, "pay_now"), payURL),
		),
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, depositMsg)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
	
	logger.Info("Custom deposit order created", "user_id", user.ID, "amount_cents", amountCents, "order_id", order.ID)
}

// escapeMarkdown escapes special characters for Telegram Markdown
func escapeMarkdown(text string) string {
	// Characters that need to be escaped in Telegram Markdown
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

func (b *Bot) handleSupportCommand(message *tgbotapi.Message) {
	// Get user for language
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}

	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)

	// Check if ticket service is available
	if b.ticketService == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 客服系统暂时不可用 / Support system is temporarily unavailable")
		b.api.Send(msg)
		return
	}

	// Check if user already has an active ticket
	ticket, err := b.ticketService.GetTicketByUserMessage(message.From.ID)
	if err == nil && ticket != nil {
		// User has an active ticket
		msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Format(lang, "ticket_already_open", map[string]interface{}{
			"TicketID": ticket.TicketID,
		}))

		// If template not found, use fallback
		if msg.Text == "ticket_already_open" {
			msg.Text = fmt.Sprintf("您已有一个进行中的工单：%s\n请直接发送消息继续对话。\n\nYou already have an open ticket: %s\nJust send messages to continue the conversation.", ticket.TicketID, ticket.TicketID)
		}

		b.api.Send(msg)
		return
	}

	// Create new ticket
	username := message.From.UserName
	if username == "" {
		username = fmt.Sprintf("User %d", message.From.ID)
	}

	newTicket, err := b.ticketService.CreateTicket(
		message.From.ID,
		username,
		"用户咨询 / User Inquiry",
		"general",
		"用户发起了新的客服对话 / User initiated a support conversation",
	)

	if err != nil {
		logger.Error("Failed to create ticket", "error", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 创建工单失败，请稍后再试 / Failed to create ticket, please try again later")
		b.api.Send(msg)
		return
	}

	// Send confirmation to user
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Format(lang, "ticket_created", map[string]interface{}{
		"TicketID": newTicket.TicketID,
	}))

	// If template not found, use fallback
	if msg.Text == "ticket_created" {
		msg.Text = fmt.Sprintf("✅ 工单已创建：%s\n\n请发送您的问题，我们的客服人员会尽快回复您。\n\n✅ Ticket created: %s\n\nPlease send your question, our support staff will reply as soon as possible.", newTicket.TicketID, newTicket.TicketID)
	}

	b.api.Send(msg)
}

func (b *Bot) handleSupportButton(message *tgbotapi.Message) {
	// Get user for language
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}

	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	logger.Info("Support button clicked", "user_id", user.ID, "tg_user_id", message.From.ID, "lang", lang)

	// Check if ticket service is available
	if b.ticketService == nil {
		logger.Error("Ticket service is nil")
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ 客服系统暂时不可用 / Support system is temporarily unavailable")
		b.api.Send(msg)
		return
	}

	// Check if user already has an active ticket
	ticket, err := b.ticketService.GetTicketByUserMessage(message.From.ID)
	if err == nil && ticket != nil {
		// User has an active ticket - show current status
		logger.Info("User already has active ticket", "ticket_id", ticket.ID, "ticket_number", ticket.TicketID)
		msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Format(lang, "ticket_already_open", map[string]interface{}{
			"TicketID": ticket.TicketID,
		}))
		b.api.Send(msg)
		return
	}

	// Show support welcome message and create ticket
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "support_welcome"))
	b.api.Send(msg)

	// Create new ticket
	username := message.From.UserName
	if username == "" {
		username = fmt.Sprintf("User %d", message.From.ID)
	}

	logger.Info("Creating new ticket", "user_id", message.From.ID, "username", username)

	newTicket, err := b.ticketService.CreateTicket(
		message.From.ID,
		username,
		"用户咨询 / User Inquiry",
		"general",
		"用户点击了客服支持按钮 / User clicked support button",
	)

	if err != nil {
		logger.Error("Failed to create ticket", "error", err)
		errMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ 创建工单失败，请稍后再试 / Failed to create ticket, please try again later")
		b.api.Send(errMsg)
		return
	}

	logger.Info("Ticket created successfully", "ticket_id", newTicket.ID, "ticket_number", newTicket.TicketID)

	// Send ticket creation confirmation
	confirmMsg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Format(lang, "support_ticket_created", map[string]interface{}{
		"TicketID": newTicket.TicketID,
	}))
	b.api.Send(confirmMsg)
}

// isAdminReplyToTicket checks if the message is from an admin replying to a ticket notification
func (b *Bot) isAdminReplyToTicket(message *tgbotapi.Message) bool {
	logger.Info("Checking if admin reply to ticket",
		"has_reply", message.ReplyToMessage != nil,
		"from_id", message.From.ID)

	// Check if message is a reply
	if message.ReplyToMessage == nil {
		return false
	}

	logger.Info("Reply message details",
		"reply_from_id", message.ReplyToMessage.From.ID,
		"bot_id", b.api.Self.ID,
		"reply_text", message.ReplyToMessage.Text)

	// Check if the replied message is from the bot
	if message.ReplyToMessage.From.ID != b.api.Self.ID {
		return false
	}

	// Check if the replied message contains ticket notification pattern
	replyText := message.ReplyToMessage.Text
	isTicketNotification := strings.Contains(replyText, "💬 *工单回复提醒*") ||
		strings.Contains(replyText, "🎫 *新工单提醒*") ||
		strings.Contains(replyText, "工单回复提醒") ||
		strings.Contains(replyText, "新工单提醒")

	logger.Info("Ticket notification check",
		"is_ticket_notification", isTicketNotification)

	if isTicketNotification {
		// Check if sender is an admin
		var admin store.AdminUser
		telegramID := message.From.ID
		err := b.db.Where("telegram_id = ? AND is_active = true", telegramID).First(&admin).Error
		if err == nil {
			logger.Info("Admin replying to ticket notification",
				"admin_id", admin.ID,
				"admin_username", admin.Username,
				"telegram_id", telegramID,
				"reply_to", replyText)
			return true
		} else {
			logger.Info("User is not admin", "telegram_id", telegramID, "error", err)
		}
	}

	return false
}

// handleAdminTicketReply handles admin replies to ticket notifications
func (b *Bot) handleAdminTicketReply(message *tgbotapi.Message) {
	replyText := message.ReplyToMessage.Text
	logger.Info("Processing admin ticket reply", "reply_text", replyText)

	// Extract ticket ID from the notification message - handle both markdown and plain text
	// Try markdown format first
	ticketIDPattern := regexp.MustCompile(`工单号:\s*(?:\x60)?(TK-\d{8}-\d{3})(?:\x60)?`)
	matches := ticketIDPattern.FindStringSubmatch(replyText)

	if len(matches) < 2 {
		// Try plain text format
		ticketIDPattern = regexp.MustCompile(`工单号:\s*(TK-\d{8}-\d{3})`)
		matches = ticketIDPattern.FindStringSubmatch(replyText)
	}

	if len(matches) < 2 {
		logger.Error("Failed to extract ticket ID from notification", "text", replyText)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ 无法识别工单号 / Failed to identify ticket number")
		b.api.Send(errorMsg)
		return
	}

	ticketNumber := matches[1]
	logger.Info("Admin replying to ticket", "ticket_number", ticketNumber, "reply", message.Text)

	// Find the ticket
	var ticket store.Ticket
	err := b.db.Where("ticket_id = ?", ticketNumber).First(&ticket).Error
	if err != nil {
		logger.Error("Failed to find ticket", "ticket_number", ticketNumber, "error", err)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ 找不到工单 / Ticket not found")
		b.api.Send(errorMsg)
		return
	}

	// Get admin info
	var admin store.AdminUser
	telegramID := message.From.ID
	err = b.db.Where("telegram_id = ?", telegramID).First(&admin).Error
	if err != nil {
		logger.Error("Failed to find admin", "telegram_id", telegramID, "error", err)
		return
	}

	// Add admin's reply to the ticket
	err = b.ticketService.AddMessage(ticket.ID, "admin", message.From.ID, admin.Username, message.Text, message.MessageID)
	if err != nil {
		logger.Error("Failed to add admin message to ticket", "error", err, "ticket_id", ticket.ID)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ 发送失败 / Failed to send message")
		b.api.Send(errorMsg)
		return
	}

	// Send the reply to the user
	userMsg := fmt.Sprintf("💬 *客服回复 / Support Reply*\n\n%s", message.Text)
	msg := tgbotapi.NewMessage(ticket.UserID, userMsg)
	msg.ParseMode = "Markdown"

	_, err = b.api.Send(msg)
	if err != nil {
		logger.Error("Failed to send message to user", "error", err, "user_id", ticket.UserID)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ 发送失败，用户可能已停止机器人 / Failed to send, user may have blocked the bot")
		b.api.Send(errorMsg)
		return
	}

	// Send confirmation to admin
	confirmMsg := tgbotapi.NewMessage(message.Chat.ID, "✅ 消息已发送给用户 / Message sent to user")
	b.api.Send(confirmMsg)

	logger.Info("Admin reply sent to user successfully",
		"ticket_id", ticket.ID,
		"admin_id", admin.ID,
		"user_id", ticket.UserID)
}