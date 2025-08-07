package httpadmin

import (
	"context"
	
	logger "shop-bot/internal/log"
)

// sendStockUpdateNotification sends stock update broadcast
func (s *Server) sendStockUpdateNotification(productName string, newStock int) {
	if s.broadcast == nil {
		logger.Warn("Broadcast service not available, skipping stock notification")
		return
	}
	
	// Send broadcast in background
	if err := s.broadcast.BroadcastStockUpdate(productName, newStock); err != nil {
		logger.Error("Failed to send stock update broadcast", 
			"product", productName, 
			"stock", newStock,
			"error", err,
		)
	} else {
		logger.Info("Stock update broadcast sent", 
			"product", productName,
			"stock", newStock,
		)
	}
}