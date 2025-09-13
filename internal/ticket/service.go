package ticket

import (
	"fmt"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// Service handles ticket operations
type Service struct {
	db  *gorm.DB
	bot *tgbotapi.BotAPI
}

// NewService creates a new ticket service
func NewService(db *gorm.DB, bot *tgbotapi.BotAPI) *Service {
	return &Service{
		db:  db,
		bot: bot,
	}
}

// CreateTicket creates a new support ticket
func (s *Service) CreateTicket(userID int64, username, subject, category, content string) (*store.Ticket, error) {
	logger.Info("Creating ticket",
		"user_id", userID,
		"username", username,
		"subject", subject,
		"category", category,
		"content", content)

	// Generate ticket ID
	ticketID := s.generateTicketID()

	ticket := &store.Ticket{
		TicketID: ticketID,
		UserID:   userID,
		Username: username,
		Subject:  subject,
		Category: category,
		Status:   "open",
		Priority: "normal",
	}

	// Start transaction
	tx := s.db.Begin()

	// Create ticket
	if err := tx.Create(ticket).Error; err != nil {
		tx.Rollback()
		logger.Error("Failed to create ticket", "error", err)
		return nil, fmt.Errorf("failed to create ticket: %w", err)
	}

	logger.Info("Ticket created in database", "ticket_id", ticket.ID, "ticket_number", ticket.TicketID)
	
	// Create initial message
	message := &store.TicketMessage{
		TicketID:   ticket.ID,
		SenderType: "user",
		SenderID:   userID,
		SenderName: username,
		Content:    content,
	}
	
	if err := tx.Create(message).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create initial message: %w", err)
	}
	
	// Update last reply time
	now := time.Now()
	ticket.LastReplyAt = &now
	if err := tx.Save(ticket).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update ticket: %w", err)
	}
	
	tx.Commit()
	
	// Notify admins
	s.notifyAdminsNewTicket(ticket, content)
	
	return ticket, nil
}

// AddMessage adds a message to a ticket
func (s *Service) AddMessage(ticketID uint, senderType string, senderID int64, senderName, content string, messageID int) error {
	logger.Info("Adding message to ticket",
		"ticket_id", ticketID,
		"sender_type", senderType,
		"sender_id", senderID,
		"sender_name", senderName,
		"content", content,
		"message_id", messageID)

	message := &store.TicketMessage{
		TicketID:   ticketID,
		SenderType: senderType,
		SenderID:   senderID,
		SenderName: senderName,
		Content:    content,
		MessageID:  messageID,
	}

	tx := s.db.Begin()

	// Create message
	if err := tx.Create(message).Error; err != nil {
		tx.Rollback()
		logger.Error("Failed to create ticket message", "error", err)
		return fmt.Errorf("failed to create message: %w", err)
	}

	logger.Info("Message created successfully", "message_id", message.ID)
	
	// Update ticket last reply time
	now := time.Now()
	updates := map[string]interface{}{
		"last_reply_at": &now,
	}
	
	// If admin is replying, mark ticket as in progress
	if senderType == "admin" {
		var ticket store.Ticket
		if err := tx.First(&ticket, ticketID).Error; err == nil {
			if ticket.Status == "open" {
				updates["status"] = "in_progress"
			}
		}
	}
	
	if err := tx.Model(&store.Ticket{}).Where("id = ?", ticketID).Updates(updates).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update ticket: %w", err)
	}
	
	tx.Commit()
	
	// Notify the other party
	if senderType == "user" {
		s.notifyAdminsUserReply(ticketID, senderName, content)
	} else if senderType == "admin" {
		s.notifyUserAdminReply(ticketID, senderName, content)
	}
	
	return nil
}

// GetTicketByUserMessage finds a ticket by user's telegram message
func (s *Service) GetTicketByUserMessage(userID int64) (*store.Ticket, error) {
	var ticket store.Ticket
	err := s.db.Where("user_id = ? AND status IN ('open', 'in_progress')", userID).
		Order("created_at DESC").
		First(&ticket).Error
	
	if err != nil {
		return nil, err
	}
	
	return &ticket, nil
}

// UpdateTicketStatus updates the status of a ticket
func (s *Service) UpdateTicketStatus(ticketID uint, status string, adminID uint) error {
	updates := map[string]interface{}{
		"status": status,
	}
	
	now := time.Now()
	switch status {
	case "resolved":
		updates["resolved_at"] = &now
	case "closed":
		updates["closed_at"] = &now
	}
	
	if adminID > 0 {
		updates["assigned_to"] = adminID
	}
	
	return s.db.Model(&store.Ticket{}).Where("id = ?", ticketID).Updates(updates).Error
}

// GetTickets retrieves tickets with filters
func (s *Service) GetTickets(status string, limit, offset int) ([]store.Ticket, int64, error) {
	query := s.db.Model(&store.Ticket{})
	
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	
	var total int64
	query.Count(&total)
	
	var tickets []store.Ticket
	err := query.
		Preload("AssignedBy").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&tickets).Error
	
	return tickets, total, err
}

// GetTicketWithMessages retrieves a ticket with all its messages
func (s *Service) GetTicketWithMessages(ticketID uint) (*store.Ticket, error) {
	var ticket store.Ticket
	err := s.db.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC")
	}).Preload("AssignedBy").First(&ticket, ticketID).Error
	
	if err != nil {
		return nil, err
	}
	
	// Mark messages as read
	s.db.Model(&store.TicketMessage{}).
		Where("ticket_id = ? AND sender_type = 'user' AND is_read = false", ticketID).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		})
	
	return &ticket, nil
}

// generateTicketID generates a unique ticket ID
func (s *Service) generateTicketID() string {
	date := time.Now().Format("20060102")
	
	// Get today's ticket count
	var count int64
	today := time.Now().Format("2006-01-02")
	s.db.Model(&store.Ticket{}).
		Where("DATE(created_at) = ?", today).
		Count(&count)
	
	return fmt.Sprintf("TK-%s-%03d", date, count+1)
}

// notifyAdminsNewTicket notifies admins about a new ticket
func (s *Service) notifyAdminsNewTicket(ticket *store.Ticket, content string) {
	logger.Info("Starting to notify admins about new ticket",
		"ticket_id", ticket.ID,
		"ticket_number", ticket.TicketID,
		"bot_initialized", s.bot != nil)

	if s.bot == nil {
		logger.Error("Bot is not initialized, cannot send notifications")
		return
	}

	// Get admin users
	var admins []store.AdminUser
	s.db.Where("is_active = true AND receive_notifications = true").Find(&admins)

	logger.Info("Found admins for notification",
		"admin_count", len(admins))
	
	message := fmt.Sprintf(
		"🎫 *新工单提醒*\n\n"+
			"工单号: `%s`\n"+
			"用户: %s (ID: %d)\n"+
			"主题: %s\n"+
			"分类: %s\n"+
			"内容:\n%s",
		ticket.TicketID,
		ticket.Username,
		ticket.UserID,
		ticket.Subject,
		ticket.Category,
		content,
	)

	for _, admin := range admins {
		logger.Info("Processing admin for notification",
			"admin_id", admin.ID,
			"admin_username", admin.Username,
			"telegram_id", admin.TelegramID,
			"telegram_id_is_nil", admin.TelegramID == nil)

		if admin.TelegramID != nil && *admin.TelegramID > 0 {
			logger.Info("Sending notification to admin",
				"admin_id", admin.ID,
				"telegram_id", *admin.TelegramID)

			msg := tgbotapi.NewMessage(*admin.TelegramID, message)
			msg.ParseMode = "Markdown"
			msg.DisableWebPagePreview = true

			if _, err := s.bot.Send(msg); err != nil {
				logger.Error("Failed to notify admin about new ticket",
					"admin_id", admin.ID,
					"telegram_id", *admin.TelegramID,
					"error", err)
			} else {
				logger.Info("Successfully sent notification to admin",
					"admin_id", admin.ID,
					"telegram_id", *admin.TelegramID)
			}
		} else {
			logger.Warn("Admin has no telegram ID configured",
				"admin_id", admin.ID,
				"admin_username", admin.Username)
		}
	}
}

// notifyAdminsUserReply notifies admins about user reply
func (s *Service) notifyAdminsUserReply(ticketID uint, username, content string) {
	if s.bot == nil {
		return
	}
	
	var ticket store.Ticket
	if err := s.db.First(&ticket, ticketID).Error; err != nil {
		return
	}
	
	// Get assigned admin or all admins
	var admins []store.AdminUser
	if ticket.AssignedTo != nil && *ticket.AssignedTo > 0 {
		s.db.Where("id = ? AND is_active = true", *ticket.AssignedTo).Find(&admins)
	} else {
		s.db.Where("is_active = true AND receive_notifications = true").Find(&admins)
	}
	
	message := fmt.Sprintf(
		"💬 *工单回复提醒*\n\n"+
			"工单号: `%s`\n"+
			"用户 %s 回复:\n%s",
		ticket.TicketID,
		username,
		content,
	)
	
	for _, admin := range admins {
		if admin.TelegramID != nil && *admin.TelegramID > 0 {
			msg := tgbotapi.NewMessage(*admin.TelegramID, message)
			msg.ParseMode = "Markdown"
			msg.DisableWebPagePreview = true
			
			if _, err := s.bot.Send(msg); err != nil {
				logger.Error("Failed to notify admin about ticket reply",
					"admin_id", admin.ID,
					"error", err)
			}
		}
	}
}

// notifyUserAdminReply notifies user about admin reply
func (s *Service) notifyUserAdminReply(ticketID uint, adminName, content string) {
	if s.bot == nil {
		return
	}
	
	var ticket store.Ticket
	if err := s.db.First(&ticket, ticketID).Error; err != nil {
		return
	}
	
	message := fmt.Sprintf(
		"📨 *客服回复*\n\n"+
			"工单号: `%s`\n"+
			"客服 %s 回复:\n%s\n\n"+
			"回复 /ticket 继续对话",
		ticket.TicketID,
		adminName,
		content,
	)
	
	msg := tgbotapi.NewMessage(ticket.UserID, message)
	msg.ParseMode = "Markdown"
	
	if _, err := s.bot.Send(msg); err != nil {
		logger.Error("Failed to notify user about ticket reply",
			"user_id", ticket.UserID,
			"error", err)
	}
}

// GetUnreadCount gets the count of unread tickets
func (s *Service) GetUnreadCount() (int64, error) {
	var count int64
	err := s.db.Model(&store.TicketMessage{}).
		Joins("JOIN tickets ON tickets.id = ticket_messages.ticket_id").
		Where("ticket_messages.sender_type = 'user' AND ticket_messages.is_read = false").
		Where("tickets.status IN ('open', 'in_progress')").
		Count(&count).Error
	
	return count, err
}