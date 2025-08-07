package broadcast

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	"shop-bot/internal/bot/messages"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// Service handles message broadcasting
type Service struct {
	db  *gorm.DB
	bot *tgbotapi.BotAPI
	mu  sync.Mutex
}

// NewService creates a new broadcast service
func NewService(db *gorm.DB, bot *tgbotapi.BotAPI) *Service {
	return &Service{
		db:  db,
		bot: bot,
	}
}

// BroadcastOptions defines options for broadcasting
type BroadcastOptions struct {
	Type       string // stock_update, promotion, announcement
	Content    string
	TargetType string // all, users, groups
	CreatedBy  uint
}

// SendBroadcast sends a broadcast message to specified targets
func (s *Service) SendBroadcast(ctx context.Context, opts BroadcastOptions) error {
	// Create broadcast record
	broadcast, err := store.CreateBroadcastMessage(s.db, opts.Type, opts.Content, opts.TargetType, opts.CreatedBy)
	if err != nil {
		return fmt.Errorf("failed to create broadcast: %w", err)
	}

	// Start broadcasting in background
	go s.processBroadcast(context.Background(), broadcast)

	return nil
}

// processBroadcast processes a broadcast message
func (s *Service) processBroadcast(ctx context.Context, broadcast *store.BroadcastMessage) {
	// Update status to sending
	store.UpdateBroadcastStatus(s.db, broadcast.ID, "sending")

	// Get recipients based on target type
	switch broadcast.TargetType {
	case "all":
		s.sendToUsers(ctx, broadcast)
		s.sendToGroups(ctx, broadcast)
	case "users":
		s.sendToUsers(ctx, broadcast)
	case "groups":
		s.sendToGroups(ctx, broadcast)
	}

	// Update status to completed
	store.UpdateBroadcastStatus(s.db, broadcast.ID, "completed")
}

// sendToUsers sends broadcast to all users
func (s *Service) sendToUsers(ctx context.Context, broadcast *store.BroadcastMessage) {
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
				s.sendToUser(ctx, broadcast, user)
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

// sendToGroups sends broadcast to all active groups
func (s *Service) sendToGroups(ctx context.Context, broadcast *store.BroadcastMessage) {
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
				s.sendToGroup(ctx, broadcast, group)
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

// sendToUser sends message to a single user
func (s *Service) sendToUser(ctx context.Context, broadcast *store.BroadcastMessage, user store.User) {
	// Get user language
	lang := messages.GetUserLanguage(user.Language, "")
	msgManager := messages.GetManager()
	
	// Format message based on type
	content := s.formatMessage(broadcast, lang)
	
	msg := tgbotapi.NewMessage(user.TgUserID, content)
	msg.ParseMode = "Markdown"
	
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

// sendToGroup sends message to a single group
func (s *Service) sendToGroup(ctx context.Context, broadcast *store.BroadcastMessage, group store.Group) {
	// Get group language
	lang := messages.GetUserLanguage(group.Language, "")
	
	// Format message based on type
	content := s.formatMessage(broadcast, lang)
	
	msg := tgbotapi.NewMessage(group.TgGroupID, content)
	msg.ParseMode = "Markdown"
	
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

// formatMessage formats broadcast message based on type and language
func (s *Service) formatMessage(broadcast *store.BroadcastMessage, lang string) string {
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

// BroadcastStockUpdate sends stock update notification
func (s *Service) BroadcastStockUpdate(productName string, newStock int) error {
	msgManager := messages.GetManager()
	
	// Create content in multiple languages
	contentZh := msgManager.Format("zh", "stock_update_content", map[string]interface{}{
		"ProductName": productName,
		"Stock":       newStock,
	})
	
	contentEn := msgManager.Format("en", "stock_update_content", map[string]interface{}{
		"ProductName": productName,
		"Stock":       newStock,
	})
	
	// Combine content
	content := fmt.Sprintf("%s\n\n%s", contentZh, contentEn)
	
	// Send broadcast
	return s.SendBroadcast(context.Background(), BroadcastOptions{
		Type:       "stock_update",
		Content:    content,
		TargetType: "all",
		CreatedBy:  1, // System user
	})
}

// GetBroadcastHistory retrieves broadcast history
func (s *Service) GetBroadcastHistory(limit, offset int) ([]store.BroadcastMessage, error) {
	var broadcasts []store.BroadcastMessage
	err := s.db.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("CreatedBy").
		Find(&broadcasts).Error
	return broadcasts, err
}