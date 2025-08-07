package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shop-bot/internal/app"
	"shop-bot/internal/config"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

func main() {
	// Initialize logger
	logger.Init()
	defer logger.Sync()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", "error", err)
	}

	// Initialize database
	db, err := store.InitDB(cfg.GetDBDSN())
	if err != nil {
		logger.Fatal("Failed to init database", "error", err)
	}

	// Seed test data
	if err := store.SeedData(db); err != nil {
		logger.Error("Failed to seed data", "error", err)
	}
	
	// Create default message templates
	if err := store.CreateDefaultTemplates(db); err != nil {
		logger.Error("Failed to create default templates", "error", err)
	}

	// Create application instance
	application, err := app.New(cfg, db)
	if err != nil {
		logger.Fatal("Failed to create application", "error", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start application
	if err := application.Start(ctx); err != nil {
		logger.Fatal("Failed to start application", "error", err)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down...")
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	
	if err := application.Shutdown(shutdownCtx); err != nil {
		logger.Error("Application shutdown error", "error", err)
	}

	// Wait for all components to finish
	application.Wait()
	
	logger.Info("Shutdown complete")
}