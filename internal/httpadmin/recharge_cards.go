package httpadmin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	
	// Get total count
	var total int64
	s.db.Model(&store.RechargeCard{}).Count(&total)
	
	// Get cards
	var cards []store.RechargeCard
	if err := s.db.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Preload("User").
		Find(&cards).Error; err != nil {
		logger.Error("Failed to fetch recharge cards", "error", err)
		c.String(http.StatusInternalServerError, "Database error")
		return
	}
	
	// Calculate pagination
	totalPages := int(total+int64(perPage)-1) / perPage
	
	c.HTML(http.StatusOK, "recharge_cards.html", gin.H{
		"cards":       cards,
		"currentPage": page,
		"totalPages":  totalPages,
		"total":       total,
		"now":         time.Now(),
	})
}

func (s *Server) handleRechargeCardGenerate(c *gin.Context) {
	var req struct {
		Count       int    `json:"count" form:"count"`
		AmountCents int    `json:"amount_cents" form:"amount_cents"`
		ExpiresIn   int    `json:"expires_in" form:"expires_in"` // Days
		Prefix      string `json:"prefix" form:"prefix"`
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be at least $1.00"})
		return
	}
	
	// Set default prefix
	if req.Prefix == "" {
		req.Prefix = "RC"
	}
	
	// Calculate expiry
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, req.ExpiresIn)
		expiresAt = &exp
	}
	
	// Generate cards
	var cards []store.RechargeCard
	for i := 0; i < req.Count; i++ {
		card := store.RechargeCard{
			Code:        store.GenerateRechargeCardCode(req.Prefix),
			AmountCents: req.AmountCents,
			ExpiresAt:   expiresAt,
			IsUsed:      false,
		}
		cards = append(cards, card)
	}
	
	// Batch insert
	if err := s.db.CreateInBatches(cards, 100).Error; err != nil {
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
	
	// Check if card is used
	var card store.RechargeCard
	if err := s.db.First(&card, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found"})
		return
	}
	
	if card.IsUsed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete used card"})
		return
	}
	
	// Delete card
	if err := s.db.Delete(&card).Error; err != nil {
		logger.Error("Failed to delete recharge card", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete card"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Card deleted successfully"})
}