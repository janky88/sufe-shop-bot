package httpadmin

import (
	"net/http"
	"strconv"
	"time"
	
	"github.com/gin-gonic/gin"
	
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// handleTicketList handles the ticket list page
func (s *Server) handleTicketList(c *gin.Context) {
	// Get query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.DefaultQuery("status", "all")
	
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	
	offset := (page - 1) * limit
	
	// Get tickets
	tickets, total, err := s.ticketService.GetTickets(status, limit, offset)
	if err != nil {
		logger.Error("Failed to get tickets", "error", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to load tickets",
		})
		return
	}
	
	// Get unread count
	unreadCount, _ := s.ticketService.GetUnreadCount()
	
	// Calculate statistics
	var stats struct {
		TotalTickets   int64
		OpenTickets    int64
		InProgress     int64
		ResolvedTickets int64
		UnreadMessages int64
	}
	
	s.db.Model(&store.Ticket{}).Count(&stats.TotalTickets)
	s.db.Model(&store.Ticket{}).Where("status = ?", "open").Count(&stats.OpenTickets)
	s.db.Model(&store.Ticket{}).Where("status = ?", "in_progress").Count(&stats.InProgress)
	s.db.Model(&store.Ticket{}).Where("status = ?", "resolved").Count(&stats.ResolvedTickets)
	stats.UnreadMessages = unreadCount
	
	c.HTML(http.StatusOK, "ticket_list.html", gin.H{
		"tickets":      tickets,
		"total":        total,
		"page":         page,
		"limit":        limit,
		"status":       status,
		"stats":        stats,
		"currentTime":  time.Now(),
	})
}

// handleTicketDetail handles the ticket detail page with conversation
func (s *Server) handleTicketDetail(c *gin.Context) {
	idStr := c.Param("id")
	ticketID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "Invalid ticket ID",
		})
		return
	}
	
	// Get ticket with messages
	ticket, err := s.ticketService.GetTicketWithMessages(uint(ticketID))
	if err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"error": "Ticket not found",
		})
		return
	}
	
	// Get quick reply templates
	var templates []store.TicketTemplate
	s.db.Where("is_active = true").Order("name ASC").Find(&templates)
	
	// Get admin list for assignment
	var admins []store.AdminUser
	s.db.Where("is_active = true").Find(&admins)
	
	c.HTML(http.StatusOK, "ticket_detail.html", gin.H{
		"ticket":    ticket,
		"templates": templates,
		"admins":    admins,
	})
}

// handleTicketReply handles admin reply to a ticket
func (s *Server) handleTicketReply(c *gin.Context) {
	idStr := c.Param("id")
	ticketID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ticket ID"})
		return
	}
	
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Get admin info from context
	adminID := c.GetUint("user_id")
	adminName := c.GetString("username")
	if adminID == 0 {
		adminID = 1 // Default admin
		adminName = "Admin"
	}
	
	// Add message to ticket
	err = s.ticketService.AddMessage(uint(ticketID), "admin", int64(adminID), adminName, req.Content, 0)
	if err != nil {
		logger.Error("Failed to add ticket message", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send reply"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reply sent successfully",
	})
}

// handleTicketStatusUpdate handles ticket status update
func (s *Server) handleTicketStatusUpdate(c *gin.Context) {
	idStr := c.Param("id")
	ticketID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ticket ID"})
		return
	}
	
	var req struct {
		Status string `json:"status" binding:"required,oneof=open in_progress resolved closed"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Get admin ID from context
	adminID := c.GetUint("user_id")
	if adminID == 0 {
		adminID = 1
	}
	
	// Update ticket status
	err = s.ticketService.UpdateTicketStatus(uint(ticketID), req.Status, adminID)
	if err != nil {
		logger.Error("Failed to update ticket status", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	
	// Add system message
	statusText := map[string]string{
		"open":        "重新打开",
		"in_progress": "处理中",
		"resolved":    "已解决",
		"closed":      "已关闭",
	}
	
	systemMessage := "工单状态更新为: " + statusText[req.Status]
	s.ticketService.AddMessage(uint(ticketID), "system", 0, "System", systemMessage, 0)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Status updated successfully",
	})
}

// handleTicketAssign handles ticket assignment
func (s *Server) handleTicketAssign(c *gin.Context) {
	idStr := c.Param("id")
	ticketID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ticket ID"})
		return
	}
	
	var req struct {
		AdminID uint `json:"admin_id" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Update assignment
	err = s.db.Model(&store.Ticket{}).Where("id = ?", ticketID).Update("assigned_to", req.AdminID).Error
	if err != nil {
		logger.Error("Failed to assign ticket", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign ticket"})
		return
	}
	
	// Add system message
	var admin store.AdminUser
	s.db.First(&admin, req.AdminID)
	
	systemMessage := "工单已分配给: " + admin.Username
	s.ticketService.AddMessage(uint(ticketID), "system", 0, "System", systemMessage, 0)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Ticket assigned successfully",
	})
}

// handleTicketTemplates handles ticket template management
func (s *Server) handleTicketTemplates(c *gin.Context) {
	var templates []store.TicketTemplate
	s.db.Order("category, name").Find(&templates)
	
	c.HTML(http.StatusOK, "ticket_templates.html", gin.H{
		"templates": templates,
	})
}

// handleTicketTemplateCreate creates a new ticket template
func (s *Server) handleTicketTemplateCreate(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Category string `json:"category"`
		Content  string `json:"content" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	template := &store.TicketTemplate{
		Name:     req.Name,
		Category: req.Category,
		Content:  req.Content,
		IsActive: true,
	}
	
	if err := s.db.Create(template).Error; err != nil {
		logger.Error("Failed to create ticket template", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Template created successfully",
		"template": template,
	})
}

// handleTicketTemplateUpdate updates a ticket template
func (s *Server) handleTicketTemplateUpdate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}
	
	var req struct {
		Name     string `json:"name" binding:"required"`
		Category string `json:"category"`
		Content  string `json:"content" binding:"required"`
		IsActive bool   `json:"is_active"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	updates := map[string]interface{}{
		"name":      req.Name,
		"category":  req.Category,
		"content":   req.Content,
		"is_active": req.IsActive,
	}
	
	if err := s.db.Model(&store.TicketTemplate{}).Where("id = ?", templateID).Updates(updates).Error; err != nil {
		logger.Error("Failed to update ticket template", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Template updated successfully",
	})
}

// handleTicketTemplateDelete deletes a ticket template
func (s *Server) handleTicketTemplateDelete(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}
	
	if err := s.db.Delete(&store.TicketTemplate{}, templateID).Error; err != nil {
		logger.Error("Failed to delete ticket template", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Template deleted successfully",
	})
}