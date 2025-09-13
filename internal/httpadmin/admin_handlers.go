package httpadmin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// handleGetAdminTelegram gets admin telegram ID
func (s *Server) handleGetAdminTelegram(c *gin.Context) {
	adminID := c.GetUint("user_id")
	if adminID == 0 {
		adminID = 1 // Default admin
	}

	var admin store.AdminUser
	if err := s.db.First(&admin, adminID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Admin not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"telegram_id": admin.TelegramID,
		"receive_notifications": admin.ReceiveNotifications,
	})
}

// handleSetAdminTelegram sets admin telegram ID
func (s *Server) handleSetAdminTelegram(c *gin.Context) {
	adminID := c.GetUint("user_id")
	if adminID == 0 {
		adminID = 1 // Default admin
	}

	var req struct {
		TelegramID int64 `json:"telegram_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update admin telegram ID
	updates := map[string]interface{}{
		"telegram_id": req.TelegramID,
		"receive_notifications": true,
	}

	if err := s.db.Model(&store.AdminUser{}).Where("id = ?", adminID).Updates(updates).Error; err != nil {
		logger.Error("Failed to update admin telegram ID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update telegram ID"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Telegram ID updated successfully",
		"telegram_id": req.TelegramID,
	})
}