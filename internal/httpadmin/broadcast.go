package httpadmin

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	
	"shop-bot/internal/bot/messages"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// sendStockUpdateNotification sends stock update broadcast with product list
func (s *Server) sendStockUpdateNotification(productName string, newStock int) {
	if s.broadcast == nil {
		logger.Warn("Broadcast service not available, skipping stock notification")
		return
	}
	
	// Create stock update message
	content := fmt.Sprintf("🎉 *%s* 已上货！\n\n库存数量：%d\n\n快来选购吧！", productName, newStock)
	
	// Send broadcast with products in background
	go s.sendBroadcastWithProducts(context.Background(), "stock_update", content, "all", 1)
	
	logger.Info("Stock update broadcast with products sent", 
		"product", productName,
		"stock", newStock,
	)
}

// processBroadcastWithProducts processes a broadcast message with product inline keyboard
func (s *Server) processBroadcastWithProducts(ctx context.Context, broadcast *store.BroadcastMessage) {
	// Update status to sending
	store.UpdateBroadcastStatus(s.db, broadcast.ID, "sending")
	
	// Get active products
	products, err := store.GetActiveProducts(s.db)
	if err != nil {
		logger.Error("Failed to get products for broadcast", "error", err)
		store.UpdateBroadcastStatus(s.db, broadcast.ID, "failed")
		return
	}
	
	// Create inline keyboard with products
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, product := range products {
		// Get available stock
		stock, _ := store.CountAvailableCodes(s.db, product.ID)
		
		// Get currency symbol
		_, currencySymbol := store.GetCurrencySettings(s.db, s.config)
		
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
	
	// Get recipients based on target type
	switch broadcast.TargetType {
	case "all":
		s.sendToUsersWithKeyboard(ctx, broadcast, keyboard)
		s.sendToGroupsWithKeyboard(ctx, broadcast, keyboard)
	case "users":
		s.sendToUsersWithKeyboard(ctx, broadcast, keyboard)
	case "groups":
		s.sendToGroupsWithKeyboard(ctx, broadcast, keyboard)
	}
	
	// Update status to completed
	store.UpdateBroadcastStatus(s.db, broadcast.ID, "completed")
}

// sendToUsersWithKeyboard sends broadcast with inline keyboard to all users
func (s *Server) sendToUsersWithKeyboard(ctx context.Context, broadcast *store.BroadcastMessage, keyboard tgbotapi.InlineKeyboardMarkup) {
	users, err := store.GetAllUsers(s.db)
	if err != nil {
		logger.Error("Failed to get users for broadcast", "error", err)
		return
	}
	
	// Create worker pool
	workerCount := 10
	userChan := make(chan store.User, len(users))
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for user := range userChan {
				s.sendToUserWithKeyboard(ctx, broadcast, user, keyboard)
			}
		}()
	}
	
	// Send users to channel
	for _, user := range users {
		userChan <- user
	}
	close(userChan)
	
	wg.Wait()
}

// sendToGroupsWithKeyboard sends broadcast with inline keyboard to all active groups
func (s *Server) sendToGroupsWithKeyboard(ctx context.Context, broadcast *store.BroadcastMessage, keyboard tgbotapi.InlineKeyboardMarkup) {
	groups, err := store.GetGroupsForBroadcast(s.db, broadcast.Type)
	if err != nil {
		logger.Error("Failed to get groups for broadcast", "error", err)
		return
	}
	
	// Create worker pool
	workerCount := 10
	groupChan := make(chan store.Group, len(groups))
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range groupChan {
				s.sendToGroupWithKeyboard(ctx, broadcast, group, keyboard)
			}
		}()
	}
	
	// Send groups to channel
	for _, group := range groups {
		groupChan <- group
	}
	close(groupChan)
	
	wg.Wait()
}

// sendToUserWithKeyboard sends message with inline keyboard to a single user
func (s *Server) sendToUserWithKeyboard(ctx context.Context, broadcast *store.BroadcastMessage, user store.User, keyboard tgbotapi.InlineKeyboardMarkup) {
	if s.bot == nil {
		logger.Error("Bot not initialized")
		return
	}
	
	// Get user language
	lang := messages.GetUserLanguage(user.Language, "")
	
	// Format message based on type
	content := s.formatBroadcastMessage(broadcast, lang)
	
	msg := tgbotapi.NewMessage(user.TgUserID, content)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	
	_, err := s.bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send broadcast to user", 
			"user_id", user.TgUserID, 
			"error", err,
		)
		store.IncrementBroadcastCount(s.db, broadcast.ID, false)
		store.LogBroadcastAttempt(s.db, broadcast.ID, "user", user.TgUserID, "failed", err.Error())
	} else {
		store.IncrementBroadcastCount(s.db, broadcast.ID, true)
		store.LogBroadcastAttempt(s.db, broadcast.ID, "user", user.TgUserID, "sent", "")
	}
	
	// Rate limiting
	time.Sleep(50 * time.Millisecond)
}

// sendToGroupWithKeyboard sends message with inline keyboard to a single group
func (s *Server) sendToGroupWithKeyboard(ctx context.Context, broadcast *store.BroadcastMessage, group store.Group, keyboard tgbotapi.InlineKeyboardMarkup) {
	if s.bot == nil {
		logger.Error("Bot not initialized")
		return
	}
	
	// Get group language
	lang := messages.GetUserLanguage(group.Language, "")
	
	// Format message based on type
	content := s.formatBroadcastMessage(broadcast, lang)
	
	msg := tgbotapi.NewMessage(group.TgGroupID, content)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	
	_, err := s.bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send broadcast to group", 
			"group_id", group.TgGroupID, 
			"error", err,
		)
		store.IncrementBroadcastCount(s.db, broadcast.ID, false)
		store.LogBroadcastAttempt(s.db, broadcast.ID, "group", group.TgGroupID, "failed", err.Error())
	} else {
		store.IncrementBroadcastCount(s.db, broadcast.ID, true)
		store.LogBroadcastAttempt(s.db, broadcast.ID, "group", group.TgGroupID, "sent", "")
	}
	
	// Rate limiting
	time.Sleep(50 * time.Millisecond)
}

// formatBroadcastMessage formats broadcast message based on type and language
func (s *Server) formatBroadcastMessage(broadcast *store.BroadcastMessage, lang string) string {
	msgManager := messages.GetManager()
	
	// Add header based on broadcast type
	var header string
	switch broadcast.Type {
	case "stock_update":
		header = msgManager.Get(lang, "broadcast_stock_update")
	case "promotion":
		header = msgManager.Get(lang, "broadcast_promotion")
	case "announcement":
		header = msgManager.Get(lang, "broadcast_announcement")
	default:
		header = msgManager.Get(lang, "broadcast_message")
	}
	
	return fmt.Sprintf("%s\n\n%s", header, broadcast.Content)
}