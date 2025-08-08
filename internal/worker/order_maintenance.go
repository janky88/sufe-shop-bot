package worker

import (
	"context"
	"time"

	"gorm.io/gorm"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// OrderMaintenanceWorker handles order expiration and cleanup
type OrderMaintenanceWorker struct {
	db              *gorm.DB
	expireTicker    *time.Ticker
	cleanupTicker   *time.Ticker
	done            chan bool
}

// NewOrderMaintenanceWorker creates a new order maintenance worker
func NewOrderMaintenanceWorker(db *gorm.DB) *OrderMaintenanceWorker {
	return &OrderMaintenanceWorker{
		db:   db,
		done: make(chan bool),
	}
}

// Start begins the maintenance tasks
func (w *OrderMaintenanceWorker) Start(ctx context.Context) {
	logger.Info("Starting order maintenance worker")
	
	// Run immediately on start
	w.runExpiration()
	w.runCleanup()
	
	// Set up tickers
	w.expireTicker = time.NewTicker(1 * time.Hour)  // Check every hour
	w.cleanupTicker = time.NewTicker(24 * time.Hour) // Clean up daily
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Info("Order maintenance worker stopping due to context cancellation")
				w.Stop()
				return
			case <-w.expireTicker.C:
				w.runExpiration()
			case <-w.cleanupTicker.C:
				w.runCleanup()
			case <-w.done:
				logger.Info("Order maintenance worker stopped")
				return
			}
		}
	}()
}

// Stop halts the maintenance tasks
func (w *OrderMaintenanceWorker) Stop() {
	if w.expireTicker != nil {
		w.expireTicker.Stop()
	}
	if w.cleanupTicker != nil {
		w.cleanupTicker.Stop()
	}
	close(w.done)
}

// runExpiration executes order expiration
func (w *OrderMaintenanceWorker) runExpiration() {
	logger.Info("Running order expiration check")
	if err := store.ExpirePendingOrders(w.db); err != nil {
		logger.Error("Failed to expire orders", "error", err)
	}
}

// runCleanup executes order cleanup
func (w *OrderMaintenanceWorker) runCleanup() {
	logger.Info("Running order cleanup")
	if err := store.CleanupExpiredOrders(w.db); err != nil {
		logger.Error("Failed to cleanup orders", "error", err)
	}
}