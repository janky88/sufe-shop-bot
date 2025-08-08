package httpadmin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	
	"github.com/gin-gonic/gin"
	
	"shop-bot/internal/broadcast"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// handleBroadcastList shows the broadcast management page
func (s *Server) handleBroadcastList(c *gin.Context) {
	// Get broadcast history
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 20
	offset := (page - 1) * limit
	
	// Get broadcasts from database
	var broadcasts []store.BroadcastMessage
	var total int64
	
	// Count total
	s.db.Model(&store.BroadcastMessage{}).Count(&total)
	
	// Get broadcasts with pagination
	if err := s.db.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("CreatedBy").
		Find(&broadcasts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Get statistics
	var stats struct {
		TotalUsers  int64
		TotalGroups int64
	}
	s.db.Model(&store.User{}).Count(&stats.TotalUsers)
	s.db.Model(&store.Group{}).Where("is_active = ?", true).Count(&stats.TotalGroups)
	
	// HTML response
	c.HTML(http.StatusOK, "broadcast_list.html", gin.H{
		"broadcasts": broadcasts,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"stats":      stats,
	})
}

// handleBroadcastCreate creates a new broadcast with product list support
func (s *Server) handleBroadcastCreate(c *gin.Context) {
	var req struct {
		Type          string `form:"type" json:"type" binding:"required"`
		Content       string `form:"content" json:"content" binding:"required"`
		TargetType    string `form:"target_type" json:"target_type" binding:"required"`
		IncludeProducts bool `form:"include_products" json:"include_products"`
	}
	
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Get current admin user ID (you might want to implement proper session management)
	adminUserID := uint(1) // Default to system user
	
	// If include products is enabled, we'll send a special broadcast that includes product buttons
	if req.IncludeProducts {
		// Send broadcast with product list
		err := s.sendBroadcastWithProducts(c.Request.Context(), req.Type, req.Content, req.TargetType, adminUserID)
		if err != nil {
			logger.Error("Failed to send broadcast with products", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send broadcast"})
			return
		}
	} else {
		// Send regular broadcast
		if s.broadcast == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Broadcast service not available"})
			return
		}
		
		err := s.broadcast.SendBroadcast(c.Request.Context(), broadcast.BroadcastOptions{
			Type:       req.Type,
			Content:    req.Content,
			TargetType: req.TargetType,
			CreatedBy:  adminUserID,
		})
		
		if err != nil {
			logger.Error("Failed to create broadcast", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create broadcast"})
			return
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "广播消息已发送"})
}

// handleBroadcastDetail shows broadcast details
func (s *Server) handleBroadcastDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	// Get broadcast
	var broadcast store.BroadcastMessage
	if err := s.db.Preload("CreatedBy").First(&broadcast, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Broadcast not found"})
		return
	}
	
	// Get logs
	var logs []store.BroadcastLog
	s.db.Where("broadcast_id = ?", id).
		Order("created_at DESC").
		Limit(100).
		Find(&logs)
	
	// Calculate success rate
	var successCount int64
	s.db.Model(&store.BroadcastLog{}).
		Where("broadcast_id = ? AND status = ?", id, "sent").
		Count(&successCount)
		
	successRate := 0.0
	if broadcast.TotalRecipients > 0 {
		successRate = float64(successCount) / float64(broadcast.TotalRecipients) * 100
	}
	
	c.HTML(http.StatusOK, "broadcast_detail.html", gin.H{
		"broadcast":   broadcast,
		"logs":        logs,
		"successRate": successRate,
	})
}

// sendBroadcastWithProducts sends a broadcast message with product inline keyboard
func (s *Server) sendBroadcastWithProducts(ctx context.Context, msgType, content, targetType string, createdBy uint) error {
	// Create broadcast record
	broadcast, err := store.CreateBroadcastMessage(s.db, msgType, content, targetType, createdBy)
	if err != nil {
		return fmt.Errorf("failed to create broadcast: %w", err)
	}
	
	// Start broadcasting with products in background
	go s.processBroadcastWithProducts(context.Background(), broadcast)
	
	return nil
}