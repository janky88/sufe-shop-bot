package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
	"shop-bot/internal/payment/epay"
	"shop-bot/internal/config"
	"shop-bot/internal/bot/messages"
	"shop-bot/internal/metrics"
	"shop-bot/internal/broadcast"
	"gorm.io/gorm"
)

type Bot struct {
	api       *tgbotapi.BotAPI
	db        *gorm.DB
	epay      *epay.Client
	config    *config.Config
	msg       *messages.Manager
	broadcast *broadcast.Service
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

	return &Bot{
		api:    api,
		db:     db,
		epay:   epayClient,
		config: cfg,
		msg:    messages.GetManager(),
		broadcast: broadcast.NewService(db, api),
	}, nil
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
		b.handleTextMessage(update.Message)
	}
}

func (b *Bot) handleStart(message *tgbotapi.Message) {
	// Get or create user
	langCode := message.From.LanguageCode
	user, err := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	if err != nil {
		logger.Error("Failed to get/create user", "error", err, "tg_user_id", message.From.ID)
		return
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
	
	// Check against localized button texts
	switch message.Text {
	case b.msg.Get(lang, "btn_buy"), "Buy":
		b.handleBuy(message)
	case b.msg.Get(lang, "btn_deposit"), "Deposit":
		b.handleDeposit(message)
	case b.msg.Get(lang, "btn_profile"), "Profile":
		b.handleProfile(message)
	case b.msg.Get(lang, "btn_orders"), "Orders", "My Orders":
		b.handleMyOrders(message)
	case b.msg.Get(lang, "btn_faq"), "FAQ":
		b.handleFAQ(message)
	case "/language":
		b.handleLanguageSelection(message)
	default:
		// Check if it's a recharge card code (starts with specific prefix)
		if strings.HasPrefix(message.Text, "RC-") || strings.HasPrefix(message.Text, "充值卡-") {
			b.handleRechargeCard(message)
		}
	}
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
		buttonText := fmt.Sprintf("%s - $%.2f (%d)", 
			product.Name, 
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
	} else if strings.HasPrefix(callback.Data, "order:") {
		orderIDStr := strings.TrimPrefix(callback.Data, "order:")
		var orderID uint
		fmt.Sscanf(orderIDStr, "%d", &orderID)
		b.handleOrderDetails(callback, orderID)
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
	
	// Track order created metric
	metrics.OrdersCreated.Inc()
	
	// Generate out_trade_no
	outTradeNo := fmt.Sprintf("%d-%d", order.ID, time.Now().Unix())
	
	// Update order with out_trade_no
	if err := b.db.Model(&store.Order{}).Where("id = ?", order.ID).Update("epay_out_trade_no", outTradeNo).Error; err != nil {
		logger.Error("Failed to update order out_trade_no", "error", err, "order_id", order.ID)
	}
	
	// Check if payment is configured
	if b.epay == nil {
		orderMsg := b.msg.Format(lang, "order_created", map[string]interface{}{
			"ProductName": product.Name,
			"Price":       fmt.Sprintf("%.2f", float64(product.PriceCents)/100),
			"OrderID":     order.ID,
		})
		orderMsg += "\n\n" + b.msg.Get(lang, "payment_not_configured")
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		b.api.Send(msg)
		return
	}
	
	// Create payment order
	notifyURL := fmt.Sprintf("%s/payment/epay/notify", b.config.BaseURL)
	returnURL := fmt.Sprintf("%s/payment/return", b.config.BaseURL)
	
	// Detect client IP (in Telegram bot context, use default)
	clientIP := "127.0.0.1"
	
	// Create order with improved parameters
	resp, err := b.epay.CreateOrder(epay.CreateOrderParams{
		OutTradeNo: outTradeNo,
		Name:       product.Name,
		Money:      float64(product.PriceCents) / 100,
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		ClientIP:   clientIP,
		Device:     epay.DeviceMobile, // Most Telegram users are on mobile
		Param:      fmt.Sprintf("user_%d", user.ID), // Store user ID for reference
	})
	
	if err != nil {
		logger.Error("Failed to create payment order", "error", err, "order_id", order.ID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_payment"))
		return
	}
	
	// Get appropriate payment URL
	payURL := resp.GetPaymentURL()
	if payURL == "" {
		logger.Error("No payment URL returned", "order_id", order.ID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_payment"))
		return
	}
	
	// Send payment message with inline button
	orderMsg := b.msg.Format(lang, "order_created", map[string]interface{}{
		"ProductName": product.Name,
		"Price":       fmt.Sprintf("%.2f", float64(product.PriceCents)/100),
		"OrderID":     order.ID,
	})
	
	// Check if it's a QR code
	if resp.IsQRCode() {
		// For QR code payments, we could generate a QR image
		// For now, just send the URL with instructions
		orderMsg += "\n\n" + b.msg.Get(lang, "scan_qr_to_pay")
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		msg.ParseMode = "Markdown"
		
		// Send QR code content as monospace text
		qrMsg := fmt.Sprintf("```\n%s\n```", payURL)
		msg.Text = orderMsg + "\n\n" + qrMsg
		b.api.Send(msg)
	} else {
		// Regular payment URL
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(b.msg.Get(lang, "pay_now"), payURL),
			),
		)
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
	}
	
	logger.Info("Order created", "order_id", order.ID, "user_id", user.ID, "product_id", product.ID)
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

	// Generate out_trade_no for payment
	outTradeNo := fmt.Sprintf("%d-%d", order.ID, time.Now().Unix())

	// Update order with out_trade_no
	if err := b.db.Model(&store.Order{}).Where("id = ?", order.ID).Update("epay_out_trade_no", outTradeNo).Error; err != nil {
		logger.Error("Failed to update order out_trade_no", "error", err, "order_id", order.ID)
	}

	// Check if payment is configured
	if b.epay == nil {
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
		
		orderMsg += "\n\n" + b.msg.Get(lang, "payment_not_configured")
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		b.api.Send(msg)
		return
	}

	// Create payment order
	notifyURL := fmt.Sprintf("%s/payment/epay/notify", b.config.BaseURL)
	returnURL := fmt.Sprintf("%s/payment/return", b.config.BaseURL)

	// Detect client IP (in Telegram bot context, use default)
	clientIP := "127.0.0.1"

	// Create order with improved parameters
	resp, err := b.epay.CreateOrder(epay.CreateOrderParams{
		OutTradeNo: outTradeNo,
		Name:       product.Name,
		Money:      float64(order.PaymentAmount) / 100, // Use payment amount after balance deduction
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		ClientIP:   clientIP,
		Device:     epay.DeviceMobile, // Most Telegram users are on mobile
		Param:      fmt.Sprintf("user_%d", user.ID), // Store user ID for reference
	})

	if err != nil {
		logger.Error("Failed to create payment order", "error", err, "order_id", order.ID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_payment"))
		return
	}

	// Get appropriate payment URL
	payURL := resp.GetPaymentURL()
	if payURL == "" {
		logger.Error("No payment URL returned", "order_id", order.ID)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_create_payment"))
		return
	}

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

	// Check if it's a QR code
	if resp.IsQRCode() {
		// For QR code payments, we could generate a QR image
		// For now, just send the URL with instructions
		orderMsg += "\n\n" + b.msg.Get(lang, "scan_qr_to_pay")
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		msg.ParseMode = "Markdown"
		
		// Send QR code content as monospace text
		qrMsg := fmt.Sprintf("```\n%s\n```", payURL)
		msg.Text = orderMsg + "\n\n" + qrMsg
		b.api.Send(msg)
	} else {
		// Regular payment URL
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(b.msg.Get(lang, "pay_now"), payURL),
			),
		)
		
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, orderMsg)
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
	}

	logger.Info("Order created", "order_id", order.ID, "user_id", user.ID, "product_id", product.ID, "balance_used", order.BalanceUsed)
}

func (b *Bot) handleDeposit(message *tgbotapi.Message) {
	// Get user for language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Get current balance
	balance, _ := store.GetUserBalance(b.db, user.ID)
	
	depositMsg := b.msg.Format(lang, "deposit_info", map[string]interface{}{
		"Balance": fmt.Sprintf("%.2f", float64(balance)/100),
	})
	
	msg := tgbotapi.NewMessage(message.Chat.ID, depositMsg)
	b.api.Send(msg)
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
	
	profileMsg := b.msg.Format(lang, "profile_info", map[string]interface{}{
		"UserID":     user.TgUserID,
		"Username":   user.Username,
		"Language":   user.Language,
		"JoinedDate": user.CreatedAt.Format("2006-01-02"),
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
	
	faqContent := b.msg.Get(lang, "faq_content")
	faqTitle := b.msg.Get(lang, "faq_title")
	
	msg := tgbotapi.NewMessage(message.Chat.ID, faqTitle+"\n\n"+faqContent)
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
		
		buttonText := fmt.Sprintf("%s - $%.2f (%d)", 
			product.Name, 
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

// GetAPI returns the underlying Telegram Bot API instance
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
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