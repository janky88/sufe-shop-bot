package httpadmin

import (
	"context"
	"net/http"
	"strconv"
	
	"github.com/gin-gonic/gin"
	
	"shop-bot/internal/broadcast"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// handleBroadcastPage shows the broadcast page
func (s *Server) handleBroadcastPage(c *gin.Context) {
	// Get statistics
	var stats struct {
		TotalUsers   int64
		TotalGroups  int64
		ActiveGroups int64
	}
	
	s.db.Model(&store.User{}).Count(&stats.TotalUsers)
	s.db.Model(&store.Group{}).Count(&stats.TotalGroups)
	s.db.Model(&store.Group{}).Where("is_active = ?", true).Count(&stats.ActiveGroups)
	
	c.HTML(http.StatusOK, "broadcast.html", gin.H{
		"stats": stats,
	})
}

// handleBroadcastSend sends a broadcast message
func (s *Server) handleBroadcastSend(c *gin.Context) {
	var req struct {
		Type       string `json:"type" form:"type"`
		TargetType string `json:"target_type" form:"target_type"`
		Content    string `json:"content" form:"content"`
	}
	
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Validate input
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		return
	}
	
	if req.Type == "" {
		req.Type = "announcement"
	}
	
	if req.TargetType == "" {
		req.TargetType = "all"
	}
	
	// Send broadcast
	err := s.broadcast.SendBroadcast(context.Background(), broadcast.BroadcastOptions{
		Type:       req.Type,
		Content:    req.Content,
		TargetType: req.TargetType,
		CreatedBy:  1, // Admin user
	})
	
	if err != nil {
		logger.Error("Failed to send broadcast", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send broadcast"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Broadcast sent successfully",
	})
}

// handleBroadcastHistory shows broadcast history
func (s *Server) handleBroadcastHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	
	perPage := 20
	offset := (page - 1) * perPage
	
	// Get total count
	var total int64
	s.db.Model(&store.BroadcastMessage{}).Count(&total)
	
	// Get broadcasts
	var broadcasts []store.BroadcastMessage
	if err := s.db.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Preload("CreatedBy").
		Find(&broadcasts).Error; err != nil {
		logger.Error("Failed to fetch broadcasts", "error", err)
		c.String(http.StatusInternalServerError, "Database error")
		return
	}
	
	// Calculate pagination
	totalPages := int(total+int64(perPage)-1) / perPage
	
	// Return JSON for AJAX requests
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"broadcasts":   broadcasts,
			"currentPage":  page,
			"totalPages":   totalPages,
			"total":        total,
		})
		return
	}
	
	c.HTML(http.StatusOK, "broadcast_history.html", gin.H{
		"broadcasts":   broadcasts,
		"currentPage":  page,
		"totalPages":   totalPages,
		"total":        total,
	})
}