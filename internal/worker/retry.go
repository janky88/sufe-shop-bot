package worker

import (
	"context"
	"fmt"
	"time"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// RetryWorker handles retrying failed message deliveries
type RetryWorker struct {
	db       *gorm.DB
	bot      *tgbotapi.BotAPI
	interval time.Duration
	maxRetries int
}

// NewRetryWorker creates a new retry worker
func NewRetryWorker(db *gorm.DB, bot *tgbotapi.BotAPI) *RetryWorker {
	return &RetryWorker{
		db:         db,
		bot:        bot,
		interval:   5 * time.Minute,
		maxRetries: 3,
	}
}

// Start starts the retry worker
func (w *RetryWorker) Start(ctx context.Context) {
	logger.Info("Starting retry worker", "interval", w.interval)
	
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	
	// Run immediately on start
	w.processFailedDeliveries()
	
	for {
		select {
		case <-ctx.Done():
			logger.Info("Retry worker stopped")
			return
		case <-ticker.C:
			w.processFailedDeliveries()
		}
	}
}

func (w *RetryWorker) processFailedDeliveries() {
	// Find orders that need delivery retry
	var orders []store.Order
	
	// Get failed delivery orders that haven't exceeded max retries
	err := w.db.Preload("User").Preload("Product").
		Where("status = ? AND delivery_retries < ?", "failed_delivery", w.maxRetries).
		Where("last_retry_at IS NULL OR last_retry_at < ?", time.Now().Add(-5*time.Minute)).
		Find(&orders).Error
		
	if err != nil {
		logger.Error("Failed to fetch orders for retry", "error", err)
		return
	}
	
	if len(orders) == 0 {
		return
	}
	
	logger.Info("Processing failed deliveries", "count", len(orders))
	
	for _, order := range orders {
		w.retryDelivery(&order)
	}
}

func (w *RetryWorker) retryDelivery(order *store.Order) {
	logger.Info("Retrying delivery", "order_id", order.ID, "retry_count", order.DeliveryRetries)
	
	// Get the code associated with this order
	var code store.Code
	err := w.db.Where("order_id = ?", order.ID).First(&code).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No code found, might be a no-stock situation
			w.handleNoStockRetry(order)
			return
		}
		logger.Error("Failed to get code for order", "order_id", order.ID, "error", err)
		return
	}
	
	// Try to send the code again
	if err := w.sendCodeToUser(order, code.Code); err != nil {
		// Update retry count and timestamp
		now := time.Now()
		updates := map[string]interface{}{
			"delivery_retries": order.DeliveryRetries + 1,
			"last_retry_at":    &now,
		}
		
		// If max retries exceeded, mark as permanently failed
		if order.DeliveryRetries+1 >= w.maxRetries {
			updates["status"] = "delivery_failed_permanent"
			logger.Error("Max retries exceeded, marking as permanent failure", "order_id", order.ID)
		}
		
		w.db.Model(order).Updates(updates)
	} else {
		// Delivery successful, update status
		w.db.Model(order).Update("status", "delivered")
		logger.Info("Delivery retry successful", "order_id", order.ID)
	}
}

func (w *RetryWorker) handleNoStockRetry(order *store.Order) {
	// Skip if this is a deposit order
	if order.ProductID == nil {
		return
	}
	
	// For no-stock orders, we might want to check if stock is now available
	stock, err := store.CountAvailableCodes(w.db, *order.ProductID)
	if err != nil {
		logger.Error("Failed to check stock", "order_id", order.ID, "error", err)
		return
	}
	
	if stock > 0 {
		// Stock is now available, try to claim and deliver
		ctx := context.Background()
		code, err := store.ClaimOneCodeTx(ctx, w.db, *order.ProductID, order.ID)
		if err == nil {
			// Successfully claimed code, deliver it
			if err := w.sendCodeToUser(order, code); err == nil {
				w.db.Model(order).Update("status", "delivered")
				logger.Info("No-stock order fulfilled after retry", "order_id", order.ID)
			}
		}
	}
}

func (w *RetryWorker) sendCodeToUser(order *store.Order, code string) error {
	// Get message template
	tmpl, err := store.GetMessageTemplate(w.db, "order_paid", order.User.Language)
	if err != nil {
		logger.Error("Failed to get message template", "error", err)
		// Use default message
		tmpl = &store.MessageTemplate{
			Content: "🎉 Payment successful!\n\nOrder ID: {{.OrderID}}\nProduct: {{.ProductName}}\nCode: `{{.Code}}`\n\nThank you for your purchase!",
		}
	}
	
	// Render message
	productName := "Unknown"
	if order.Product != nil {
		productName = order.Product.Name
	}
	
	message, err := store.RenderTemplate(tmpl.Content, map[string]interface{}{
		"OrderID":     order.ID,
		"ProductName": productName,
		"Code":        code,
	})
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}
	
	// Send message
	msg := tgbotapi.NewMessage(order.User.TgUserID, message)
	msg.ParseMode = "Markdown"
	
	if _, err := w.bot.Send(msg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	
	return nil
}

// GetFailedDeliveryStats returns statistics about failed deliveries
func GetFailedDeliveryStats(db *gorm.DB) (temporary, permanent, total int64, err error) {
	err = db.Model(&store.Order{}).Where("status = ?", "failed_delivery").Count(&temporary).Error
	if err != nil {
		return
	}
	
	err = db.Model(&store.Order{}).Where("status = ?", "delivery_failed_permanent").Count(&permanent).Error
	if err != nil {
		return
	}
	
	total = temporary + permanent
	return
}