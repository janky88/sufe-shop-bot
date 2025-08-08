package httpadmin

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"shop-bot/internal/store"
	logger "shop-bot/internal/log"
)

func (s *Server) handleRechargeCardList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	
	perPage := 20
	offset := (page - 1) * perPage
	showUsed := c.Query("show_used") == "true"
	
	// Get cards with new function
	cards, total, err := store.GetRechargeCards(s.db, perPage, offset, showUsed)
	if err != nil {
		logger.Error("Failed to fetch recharge cards", "error", err)
		c.String(http.StatusInternalServerError, "Database error")
		return
	}
	
	// Get statistics
	totalCount, active, fullyUsed, expired, _ := store.GetRechargeCardStatsV2(s.db)
	
	// Calculate pagination
	totalPages := int(total+int64(perPage)-1) / perPage
	
	c.HTML(http.StatusOK, "recharge_cards.html", gin.H{
		"cards":       cards,
		"currentPage": page,
		"totalPages":  totalPages,
		"total":       total,
		"totalCount":  totalCount,
		"active":      active,
		"fullyUsed":   fullyUsed,
		"expired":     expired,
		"showUsed":    showUsed,
		"now":         time.Now(),
	})
}

func (s *Server) handleRechargeCardGenerate(c *gin.Context) {
	var req struct {
		Count          int    `json:"count" form:"count"`
		AmountCents    int    `json:"amount_cents" form:"amount_cents"`
		MaxUses        int    `json:"max_uses" form:"max_uses"`
		MaxUsesPerUser int    `json:"max_uses_per_user" form:"max_uses_per_user"`
		ExpiresIn      int    `json:"expires_in" form:"expires_in"` // Days
		Prefix         string `json:"prefix" form:"prefix"`
	}
	
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Validate input
	if req.Count < 1 || req.Count > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Count must be between 1 and 1000"})
		return
	}
	
	if req.AmountCents < 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be at least 1"})
		return
	}
	
	// Set defaults
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.MaxUsesPerUser <= 0 {
		req.MaxUsesPerUser = 1
	}
	
	// Calculate expiry
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, req.ExpiresIn)
		expiresAt = &exp
	}
	
	// Generate cards with new function
	cards, err := store.GenerateRechargeCards(s.db, req.Count, req.AmountCents, req.MaxUses, req.MaxUsesPerUser, expiresAt)
	if err != nil {
		logger.Error("Failed to generate recharge cards", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate cards"})
		return
	}
	
	// Return generated codes for download
	var codes []string
	for _, card := range cards {
		codes = append(codes, card.Code)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"count": len(cards),
		"codes": codes,
		"message": fmt.Sprintf("Generated %d recharge cards", len(cards)),
	})
}

func (s *Server) handleRechargeCardDelete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	// Use new delete function
	if err := store.DeleteRechargeCard(s.db, uint(id)); err != nil {
		logger.Error("Failed to delete recharge card", "error", err, "id", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Card deleted successfully"})
}

func (s *Server) handleRechargeCardUsage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	// Get usage details
	usages, err := store.GetRechargeCardUsages(s.db, uint(id))
	if err != nil {
		logger.Error("Failed to fetch recharge card usage", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"usages": usages})
}